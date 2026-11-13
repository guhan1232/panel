// Models.kt —— 面板 /api 响应结构对应的 data class（docs/APP.md §6/§7）。
// 字段名与后端 JSON 完全对齐；ignoreUnknownKeys 容忍后端加字段。
package cn.hanyuxin.panel.data

import kotlinx.serialization.Serializable

/** 统一响应壳：{code, msg, data}。data 在后端 omitempty 时缺省。 */
@Serializable
data class Resp<T>(val code: Int = 0, val msg: String? = null, val data: T? = null)

// ---- 鉴权 ----

@Serializable
data class LoginResp(
    val username: String = "",
    val role: String = "",
    val client: String = "",
    val token: String? = null,
    val expires_at: Long? = null,
    val need_2fa: Boolean = false,
)

@Serializable
data class Me(val username: String = "", val role: String = "")

@Serializable
data class SessionItem(
    val id: String = "",
    val ip: String = "",
    val ua: String = "",
    val client: String = "",
    val device_name: String = "",
    val current: Boolean = false,
    val expires: String = "",
)

// ---- 仪表盘 ----

@Serializable
data class Sample(
    val time: Long = 0,
    val cpu: Double = 0.0,
    val mem_pct: Double = 0.0,
    val mem_used: Long = 0,
    val mem_total: Long = 0,
    val recv: Long = 0,
    val sent: Long = 0,
    val load1: Double = 0.0,
)

@Serializable
data class DiskInfo(val mount: String = "", val fs: String = "", val total: Long = 0, val used: Long = 0, val used_pct: Double = 0.0)

@Serializable
data class HostInfo(
    val hostname: String = "",
    val os: String = "",
    val platform: String = "",
    val platform_family: String = "",
    val platform_version: String = "",
    val kernel_arch: String = "",
    val kernel_version: String = "",
    val uptime: Long = 0,
)

@Serializable
data class DashboardSummary(
    val host: HostInfo? = null,
    val current: Sample? = null,
    val history: List<Sample> = emptyList(),
    val disks: List<DiskInfo> = emptyList(),
    val mem_used: Long = 0,
    val mem_total: Long = 0,
)

@Serializable
data class NodeItem(
    val id: Long = 0,
    val name: String = "",
    val hostname: String = "",
    val os: String = "",
    val arch: String = "",
    val ip: String = "",
    val version: String = "",
    val last_seen: String = "",
)

// ---- 网站 ----

@Serializable
data class Site(
    val id: Long = 0,
    val name: String = "",
    val type: String = "",
    val root: String = "",
    val upstream: String = "",
    val domains: String = "",
    val php_socket: String = "",
    val sub_type: String = "",
    val force_https: Boolean = false,
    val enabled: Boolean = false,
    val system: Boolean = false,
)

// ---- 软件商店 ----

@Serializable
data class TplParam(val key: String = "", val label: String = "", val def: String = "", val required: Boolean = false)

@Serializable
data class AppTemplate(
    val key: String = "",
    val name: String = "",
    val desc: String = "",
    val kind: String = "",
    val params: List<TplParam> = emptyList(),
    val source: String = "",
    val default_version: String = "",
)

@Serializable
data class AppInst(
    val id: Long = 0,
    val name: String = "",
    val app_key: String = "",
    val kind: String = "",
    val tool: Boolean = false,
    val version: String = "",
    val binary: String = "",
)

// ---- 文件管理 ----

@Serializable
data class FileEntry(
    val name: String = "",
    val path: String = "",
    val size: Long = 0,
    val mode: String = "",
    val owner: String = "",
    val is_dir: Boolean = false,
    val mtime: String = "",
)

@Serializable
data class FileList(val path: String = "", val items: List<FileEntry> = emptyList())

// ---- 告警 ----

@Serializable
data class AlertEvent(
    val id: Long = 0,
    val rule_id: Long = 0,
    val rule_name: String = "",
    val metric: String = "",
    val value: String = "",
    val message: String = "",
    val created_at: String = "",
)

@Serializable
data class AlertEventsPage(val items: List<AlertEvent> = emptyList(), val total: Long = 0)

// ---- 设置 / APP 自身 ----

@Serializable
data class PanelInfo(
    val version: String = "",
    val listen: String = "",
    val tls: Boolean = false,
    val os: String = "",
    val arch: String = "",
)

@Serializable
data class AppCfg(val public_url: String = "", val apk_url: String = "", val latest_version: String = "", val release_notes: String = "")

@Serializable
data class UpdateInfo(
    val has_update: Boolean = false,
    val current: String = "",
    val latest: String = "",
    val apk_url: String = "",
    val notes: String = "",
)

// ---- 扫码登录 ----

@Serializable
data class QrCreateResp(val key: String = "", val qr_text: String = "", val expires_in: Int = 120)

/** 扫码结果（qr_text 解析）：
 *  - hanyuxin://login?…&key=…  扫码登录码（Web 登录页，扫码确认 Web 免密登录）
 *  - hanyuxin://bind?…&key=…   绑定码（Web 系统设置/仪表盘生成，绑定=添加服务器+免密登录）
 *  bind 码无独立 key 语义（key 字段承载绑定密钥），bindKey 非空时走绑定登录。 */
data class QrData(val host: String, val entrance: String, val key: String, val bindKey: String = "", val v2: Boolean) {
    companion object {
        fun parse(text: String): QrData? {
            val isBind = text.startsWith("hanyuxin://bind?")
            if (!text.startsWith("hanyuxin://login?") && !isBind) return null
            val q = text.substringAfter("?")
            val map = q.split("&").mapNotNull {
                val i = it.indexOf('=')
                if (i <= 0) null else java.net.URLDecoder.decode(it.substring(0, i), "UTF-8") to
                    java.net.URLDecoder.decode(it.substring(i + 1), "UTF-8")
            }.toMap()
            val host = map["host"]?.trim() ?: return null
            val entrance = map["entrance"]?.trim() ?: return null
            if (host.isEmpty() || entrance.isEmpty()) return null
            // key 可选：Web 扫码登录码带 key，绑定码（系统设置/仪表盘生成的 hanyuxin://login）不带——
            // 扫到绑定码时仅添加服务器，APP 登录后即完成绑定
            val key = if (isBind) "" else map["key"]?.trim() ?: ""
            val bindKey = if (isBind) map["key"]?.trim() ?: "" else ""
            return QrData(host = host, entrance = entrance, key = key, bindKey = bindKey, v2 = map["v2"] == "1")
        }
    }
}
