// Api.kt —— 各业务模块的 API 封装（复用面板现有 /api/*，见 docs/APP.md §6）。
package cn.hanyuxin.panel.data

import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.MultipartBody
import okio.BufferedSink
import java.io.File
import java.io.OutputStream

/** 当前服务器的客户端（token 从 ServerStore 取）。 */
fun clientFor(server: ServerInfo): PanelClient =
    PanelClient(server, ServerStore.token(server.host))

object Api {

    // ---- 鉴权（docs/APP.md §4.2）----

    /** APP 登录：client=app，成功返回 30 天会话 token。 */
    suspend fun login(server: ServerInfo, username: String, password: String, code: String, deviceName: String): LoginResp {
        val anon = PanelClient(server, null)
        val body = Http.json.encodeToString(
            mapOf(
                "username" to username, "password" to password, "code" to code,
                "client" to "app", "device_name" to deviceName,
            ),
        )
        val resp = anon.request<LoginResp>("POST", "/api/auth/login", body) ?: throw ApiException(-1, "登录响应为空")
        if (resp.need_2fa) return resp
        if (resp.token.isNullOrBlank()) throw ApiException(-1, "登录成功但未返回会话令牌")
        ServerStore.setToken(server.host, resp.token)
        ServerStore.upsert(server.host, server.entrance, server.name) // 确保存在
        ServerStore.setLastHost(server.host)
        Http.invalidate(server.host)
        return resp
    }

    suspend fun me(c: PanelClient): Me = c.request<Me>("GET", "/api/auth/me") ?: Me()
    suspend fun sessions(c: PanelClient): List<SessionItem> = c.request<List<SessionItem>>("GET", "/api/auth/sessions") ?: emptyList()
    suspend fun revokeSession(c: PanelClient, id: String) { c.request<Unit>("DELETE", "/api/auth/sessions/$id") }
    suspend fun revokeOthers(c: PanelClient) { c.request<Unit>("POST", "/api/auth/sessions/revoke-others", "{}") }
    suspend fun logout(c: PanelClient, host: String) {
        runCatching { c.request<Unit>("POST", "/api/auth/logout", "{}") }
        ServerStore.setToken(host, null)
        ServerStore.setLastHost(null)
        Http.invalidate(host)
    }

    // ---- 扫码登录（docs/APP.md §5）----

    suspend fun qrConfirm(c: PanelClient, key: String) { c.request<Unit>("POST", "/api/auth/qr/confirm", Http.json.encodeToString(mapOf("key" to key))) }
    suspend fun qrApprove(c: PanelClient, key: String) { c.request<Unit>("POST", "/api/auth/qr/approve", Http.json.encodeToString(mapOf("key" to key))) }

    // ---- 仪表盘（3s 轮询与 Web 端一致）----

    suspend fun dashboard(c: PanelClient): DashboardSummary = c.request<DashboardSummary>("GET", "/api/dashboard/summary") ?: DashboardSummary()
    suspend fun nodes(c: PanelClient): List<NodeItem> = c.request<List<NodeItem>>("GET", "/api/nodes") ?: emptyList()

    // ---- 网站 ----

    suspend fun sites(c: PanelClient): List<Site> = c.request<List<Site>>("GET", "/api/sites") ?: emptyList()
    suspend fun toggleSite(c: PanelClient, id: Long) { c.request<Unit>("POST", "/api/sites/$id/toggle", "{}") }

    // ---- 软件商店 ----

    suspend fun templates(c: PanelClient): List<AppTemplate> = c.request<List<AppTemplate>>("GET", "/api/apps/templates") ?: emptyList()
    suspend fun installedApps(c: PanelClient): List<AppInst> = c.request<List<AppInst>>("GET", "/api/apps") ?: emptyList()
    suspend fun installApp(c: PanelClient, appKey: String, name: String, version: String, params: Map<String, String>) {
        val body = Http.json.encodeToString(
            mapOf("app_key" to appKey, "name" to name, "version" to version, "params" to params),
        )
        c.request<Unit>("POST", "/api/apps", body)
    }
    suspend fun uninstallApp(c: PanelClient, id: Long, force: Boolean, removeData: Boolean) {
        val body = Http.json.encodeToString(mapOf("force" to force, "remove_data" to removeData))
        c.request<Unit>("POST", "/api/apps/$id/uninstall", body)
    }
    suspend fun upgradeApp(c: PanelClient, id: Long) { c.request<Unit>("POST", "/api/apps/$id/upgrade", "{}") }

    // ---- 文件管理 ----

    suspend fun listFiles(c: PanelClient, path: String): FileList =
        c.request<FileList>("GET", "/api/files/list", query = listOf("path" to path)) ?: FileList(path = path)

    suspend fun mkdirFile(c: PanelClient, path: String, name: String) {
        c.request<Unit>("POST", "/api/files/mkdir", Http.json.encodeToString(mapOf("path" to path, "name" to name)))
    }

    suspend fun renameFile(c: PanelClient, path: String, name: String) {
        c.request<Unit>("POST", "/api/files/rename", Http.json.encodeToString(mapOf("path" to path, "name" to name)))
    }

    suspend fun deleteFiles(c: PanelClient, paths: List<String>) {
        c.request<Unit>("POST", "/api/files/delete", Http.json.encodeToString(mapOf("paths" to paths)))
    }

    suspend fun readFile(c: PanelClient, path: String): String {
        val r = c.request<Map<String, String>>("GET", "/api/files/read", query = listOf("path" to path))
        return r?.get("content") ?: ""
    }

    suspend fun saveFile(c: PanelClient, path: String, content: String) {
        c.request<Unit>("POST", "/api/files/save", Http.json.encodeToString(mapOf("path" to path, "content" to content)))
    }

    /** 上传文件（multipart，流式写入不整读内存）。 */
    suspend fun uploadFile(c: PanelClient, dir: String, file: File, onProgress: (Long) -> Unit = {}) {
        val body = MultipartBody.Builder()
            .setType(MultipartBody.FORM)
            .addFormDataPart("path", dir)
            .addFormDataPart(
                "file", file.name,
                object : okhttp3.RequestBody() {
                    override fun contentType() = "application/octet-stream".toMediaType()
                    override fun contentLength(): Long = file.length()
                    override fun writeTo(sink: BufferedSink) {
                        file.inputStream().use { input ->
                            val buf = ByteArray(64 * 1024)
                            var sent = 0L
                            while (true) {
                                val n = input.read(buf)
                                if (n < 0) break
                                sink.write(buf, 0, n)
                                sent += n
                                onProgress(sent)
                            }
                        }
                    }
                },
            )
            .build()
        val raw = c.requestRaw("POST", "/api/files/upload", body)
        if (!raw.isSuccessful) throw ApiException(raw.code, "上传失败 HTTP " + raw.code)
        val parsed = runCatching { Http.json.decodeFromString<Resp<Unit>>(raw.text) }.getOrNull()
        if (parsed != null && parsed.code != 0) throw ApiException(parsed.code, parsed.msg ?: "上传失败")
    }

    /** 下载文件到输出流（SAF 目标流），回调进度。 */
    suspend fun downloadFile(c: PanelClient, path: String, out: OutputStream, onProgress: (Long, Long) -> Unit = { _, _ -> }) {
        val resp = c.streamRaw("GET", "/api/files/download", query = listOf("path" to path))
        resp.use { r ->
            if (!r.isSuccessful) throw ApiException(r.code, "下载失败 HTTP ${r.code}")
            val total = r.body?.contentLength() ?: -1
            val src = r.body?.byteStream() ?: throw ApiException(-1, "空响应")
            val buf = ByteArray(64 * 1024)
            var sent = 0L
            while (true) {
                val n = src.read(buf)
                if (n < 0) break
                out.write(buf, 0, n)
                sent += n
                onProgress(sent, total)
            }
            out.flush()
        }
    }

    // ---- 告警 ----

    suspend fun alertEvents(c: PanelClient, page: Int = 1, size: Int = 50): AlertEventsPage =
        c.request<AlertEventsPage>("GET", "/api/alerts/events", query = listOf("page" to page.toString(), "size" to size.toString()))
            ?: AlertEventsPage()

    // ---- 设置 / 自更新 ----

    suspend fun panelInfo(c: PanelClient): PanelInfo = c.request<PanelInfo>("GET", "/api/setting/info") ?: PanelInfo()
    suspend fun appConfig(c: PanelClient): AppCfg = c.request<AppCfg>("GET", "/api/setting/app-config") ?: AppCfg()
    suspend fun checkUpdate(server: ServerInfo): UpdateInfo {
        val anon = PanelClient(server, null)
        return anon.request<UpdateInfo>("GET", "/api/app/check-update", query = listOf("platform" to "android", "v" to "1.0.0")) ?: UpdateInfo()
    }
}
