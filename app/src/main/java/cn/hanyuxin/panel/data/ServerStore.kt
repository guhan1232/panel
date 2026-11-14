// ServerStore.kt —— 多服务器本地存储：服务器元数据 + 会话令牌 + TOFU 指纹。
// 全部落 EncryptedSharedPreferences（androidx.security-crypto），见 docs/APP.md §4.3/§8.4。
package cn.hanyuxin.panel.data

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.serialization.Serializable
import kotlinx.coroutines.flow.StateFlow
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.Json

@Serializable
data class ServerInfo(
    val host: String, // 如 https://154.8.165.155:27958 或 https://panel.hanyuxin.cn
    val entrance: String = "",
    val name: String = "",
    val addedAt: Long = 0,
    val certPin: String = "", // TOFU：首次连接记录的服务器公钥指纹（SPKI SHA-256 Base64）
    val bindKey: String = "", // 绑定密钥（扫码绑定时持久化）：令牌失效后用它免密重连，无需账号密码
)

object ServerStore {
    private const val KEY_SERVERS = "servers_json"
    private const val KEY_LAST = "last_host"
    private const val KEY_BIO = "bio_enabled"
    private const val KEY_TOTP = "totp_secret" // 2FA 验证器密钥（APP 即验证器）
    private const val KEY_TOKEN = "token:"

    private val json = Json { ignoreUnknownKeys = true }
    private var prefs: SharedPreferences? = null
    private val _servers = MutableStateFlow<List<ServerInfo>>(emptyList())
    val servers: StateFlow<List<ServerInfo>> get() = _servers

    fun init(context: Context) {
        if (prefs != null) return
        synchronized(this) {
            if (prefs != null) return
            val master = MasterKey.Builder(context)
                .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
                .build()
            prefs = EncryptedSharedPreferences.create(
                context,
                "hanyuxin_panel_sec",
                master,
                EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
                EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
            )
            _servers.value = prefs!!.getString(KEY_SERVERS, null)?.let { raw ->
                runCatching { json.decodeFromString(ListSerializer(ServerInfo.serializer()), raw) }.getOrNull()
            } ?: emptyList()
        }
    }

    private fun persist(list: List<ServerInfo>) {
        _servers.value = list
        prefs?.edit()?.putString(KEY_SERVERS, json.encodeToString(ListSerializer(ServerInfo.serializer()), list))?.apply()
    }

    fun list(): List<ServerInfo> = _servers.value
    fun byHost(host: String): ServerInfo? = _servers.value.firstOrNull { it.host == host }

    /** 添加或更新服务器（保留已有指纹与绑定密钥）；host 归一去尾斜杠。 */
    fun upsert(host: String, entrance: String, name: String = "", bindKey: String = ""): ServerInfo {
        val h = host.trim().trimEnd('/')
        val existing = byHost(h)
        val rec = ServerInfo(
            host = h,
            entrance = entrance.ifBlank { existing?.entrance ?: "" },
            name = name.ifBlank { existing?.name ?: h },
            addedAt = existing?.addedAt ?: System.currentTimeMillis(),
            certPin = existing?.certPin ?: "",
            bindKey = bindKey.ifBlank { existing?.bindKey ?: "" },
        )
        persist(listOf(rec) + _servers.value.filterNot { it.host == h })
        return rec
    }

    fun remove(host: String) {
        persist(_servers.value.filterNot { it.host == host })
        prefs?.edit()?.remove(KEY_TOKEN + host)?.apply()
        if (lastHost() == host) setLastHost(null)
    }

    /** 重置 TOFU 信任（服务器换证书后由用户主动触发）。 */
    fun resetPin(host: String) {
        val cur = byHost(host) ?: return
        persist(_servers.value.map { if (it.host == host) it.copy(certPin = "") else it })
    }

    fun pin(host: String): String? = byHost(host)?.certPin?.takeIf { it.isNotBlank() }

    fun setPin(host: String, pin: String) {
        synchronized(this) {
            val cur = _servers.value
            val target = cur.firstOrNull { it.host == host } ?: return
            if (target.certPin == pin) return
            persist(cur.map { if (it.host == host) it.copy(certPin = pin) else it })
        }
    }

    
    /** 2FA 验证器密钥（APP 自身作为验证器；单账号面板场景全局一枚）。 */
    fun totpSecret(): String = prefs?.getString(KEY_TOTP, null)?.takeIf { it.isNotBlank() } ?: ""
    fun setTotpSecret(secret: String) {
        prefs?.edit()?.putString(KEY_TOTP, secret.trim())?.apply()
    }
fun token(host: String): String? = prefs?.getString(KEY_TOKEN + host, null)?.takeIf { it.isNotBlank() }
    fun setToken(host: String, token: String?) {
        prefs?.edit()?.let { e ->
            if (token.isNullOrBlank()) e.remove(KEY_TOKEN + host) else e.putString(KEY_TOKEN + host, token)
            e.apply()
        }
    }

    fun lastHost(): String? = prefs?.getString(KEY_LAST, null)
    fun setLastHost(host: String?) {
        prefs?.edit()?.let { e -> if (host == null) e.remove(KEY_LAST) else e.putString(KEY_LAST, host); e.apply() }
    }

    fun bioEnabled(): Boolean = prefs?.getBoolean(KEY_BIO, false) ?: false
    fun setBioEnabled(v: Boolean) {
        prefs?.edit()?.putBoolean(KEY_BIO, v)?.apply()
    }
}
