// ApiClient.kt —— OkHttp 网络层：Bearer 会话 + 入口 Cookie + 自签证书 TOFU 指纹锁定。
// 安全设计见 docs/APP.md §8.4：首次连接记录服务器公钥指纹，后续连接指纹不一致立即断连。
package cn.hanyuxin.panel.data

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import okio.BufferedSink
import okio.source
import java.io.InputStream
import java.security.MessageDigest
import java.security.SecureRandom
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import java.util.Base64
import java.util.concurrent.ConcurrentHashMap
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManager
import javax.net.ssl.X509TrustManager

/** 业务错误：面板返回 code != 0。 */
class ApiException(val bizCode: Int, message: String) : RuntimeException(message)

/** 会话失效 / 未登录（APP 收到后应回到登录页）。 */
class UnauthorizedException(message: String = "未登录或会话已过期") : RuntimeException(message)

/** TOFU 指纹不匹配（疑似中间人 / 服务器换证书）。 */
class PinMismatchException(message: String) : RuntimeException(message)

/**
 * TOFU 信任管理器：不做系统 CA 校验（面板大量自签），以「首见指纹」为信任锚。
 * - 首次连接：记录 SPKI SHA-256 指纹并放行
 * - 后续连接：指纹一致才放行；不一致抛 PinMismatchException
 */
private class TofuTrustManager(
    private val pinProvider: () -> String?,
    private val pinSetter: (String) -> Unit,
) : X509TrustManager {
    override fun checkClientTrusted(chain: Array<X509Certificate>?, authType: String?) {}

    override fun checkServerTrusted(chain: Array<X509Certificate>?, authType: String?) {
        val leaf = chain?.firstOrNull() ?: throw CertificateException("服务器未提供证书")
        val pin = try {
            Base64.getEncoder().encodeToString(MessageDigest.getInstance("SHA-256").digest(leaf.publicKey.encoded))
        } catch (e: Exception) {
            throw CertificateException("证书指纹计算失败: ${e.message}")
        }
        val stored = pinProvider()
        if (stored.isNullOrBlank()) {
            pinSetter(pin)
            return
        }
        if (stored != pin) {
            throw CertificateException("TOFU_PIN_MISMATCH: 服务器证书已更换；若确认是服务器方换证书，请在 APP 服务器管理中重置信任")
        }
    }

    override fun getAcceptedIssuers(): Array<X509Certificate> = arrayOf()
}

/** 从流式输入上传的 RequestBody（避免整文件读入内存）。 */
class StreamBody(private val size: Long, private val opener: () -> InputStream) : RequestBody() {
    override fun contentType() = "application/octet-stream".toMediaType()
    override fun contentLength(): Long = size
    override fun writeTo(sink: BufferedSink) {
        opener().use { input -> sink.writeAll(input.source()) }
    }
}

/** 每个 (host, token) 复用一个 OkHttp 客户端；TOFU 指纹变化后失效重建。 */
object Http {
    private val clients = ConcurrentHashMap<String, OkHttpClient>()
    val json = Json { ignoreUnknownKeys = true; coerceInputValues = true; isLenient = true }

    fun invalidate(host: String) {
        clients.keys.removeAll { it.startsWith("$host|") }
    }

    fun clientFor(server: ServerInfo, token: String?): OkHttpClient {
        val key = "${server.host}|${token ?: ""}|${ServerStore.pin(server.host) ?: ""}"
        clients[key]?.let { return it }
        val tm = TofuTrustManager(
            pinProvider = { ServerStore.pin(server.host) },
            pinSetter = { pin ->
                ServerStore.setPin(server.host, pin)
                invalidate(server.host) // 指纹落库后按新 key 复用
            },
        )
        val ssl = SSLContext.getInstance("TLS")
        ssl.init(null, arrayOf<TrustManager>(tm), SecureRandom())
        val builder = OkHttpClient.Builder()
            .sslSocketFactory(ssl.socketFactory, tm)
            .hostnameVerifier { _, _ -> true } // 信任锚 = TOFU 指纹（自签证书常无匹配 SAN）
            .connectTimeout(java.time.Duration.ofSeconds(15))
            .readTimeout(java.time.Duration.ofSeconds(120))
            .addInterceptor { chain ->
                val b = chain.request().newBuilder()
                if (!token.isNullOrBlank()) {
                    b.header("Authorization", "Bearer $token")
                }
                if (server.entrance.isNotBlank()) {
                    b.header("Cookie", "wm_entrance=${server.entrance}")
                }
                b.header("Accept", "application/json")
                chain.proceed(b.build())
            }
        val c = builder.build()
        clients[key] = c
        return c
    }
}

/** 面板 API 客户端：统一请求 + 响应壳解包 + 401/指纹异常抛出。 */
class PanelClient(private val server: ServerInfo, private val token: String?) {

    private fun http(): OkHttpClient = Http.clientFor(server, token)

    private fun url(path: String, query: List<Pair<String, String>>): String {
        val base = server.host.trimEnd('/')
        val q = if (query.isEmpty()) "" else "?" + query.joinToString("&") { (k, v) ->
            "${java.net.URLEncoder.encode(k, "UTF-8")}=${java.net.URLEncoder.encode(v, "UTF-8")}"
        }
        return "$base$path$q"
    }

    suspend fun requestRaw(method: String, path: String, body: RequestBody? = null, query: List<Pair<String, String>> = emptyList()): Response =
        withContext(Dispatchers.IO) {
            val req = Request.Builder()
                .url(url(path, query))
                .method(method, body)
                .build()
            http().newCall(req).execute()
        }

    /** JSON 请求 + 解包 {code,msg,data}；失败抛 ApiException / UnauthorizedException / PinMismatchException。 */
    suspend inline fun <reified T> request(method: String, path: String, bodyJson: String? = null, query: List<Pair<String, String>> = emptyList()): T? {
        val resp = requestRaw(
            method, path,
            bodyJson?.toRequestBody("application/json; charset=utf-8".toMediaType()),
            query,
        )
        resp.use { r ->
            val text = r.body?.string().orEmpty()
            if (!r.isSuccessful) {
                if (r.code == 401) throw UnauthorizedException()
                val msg = runCatching { Http.json.decodeFromString<Resp<Unit>>(text).msg }.getOrNull()
                throw ApiException(r.code, msg ?: "HTTP ${r.code}")
            }
            val parsed = runCatching { Http.json.decodeFromString<Resp<T>>(text) }
                .getOrElse { throw ApiException(-1, "响应解析失败: ${it.message}") }
            if (parsed.code == 401) throw UnauthorizedException()
            if (parsed.code != 0) throw ApiException(parsed.code, parsed.msg ?: "请求失败")
            return parsed.data
        }
    }
}
