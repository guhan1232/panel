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
data class AppEntry(val app: AppInst = AppInst(), val status: String = "")

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
data class TwoFAStatus(val enabled: Boolean = false)

@Serializable
data class TwoFASetup(val secret: String = "", val url: String = "")

@Serializable
data class UpdateInfo(
    val has_update: Boolean = false,
    val current: String = "",
    val latest: String = "",
    val apk_url: String = "",
    val notes: String = "",
	val sha256: String = "",
)

// ---- 扩展模块（服务/运维全功能）----

/** 通用分页（任务日志等）：{total, items}。 */
@Serializable
data class PageResp<T>(val total: Long = 0, val items: List<T> = emptyList())

// 计划任务
@Serializable
data class Task(
    val id: Long = 0,
    val name: String = "",
    val type: String = "",
    val spec: String = "",
    val command: String = "",
    val node_id: Long = 0,
    val timeout_sec: Int = 0,
    val enabled: Boolean = false,
    val last_run_at: String? = null,
    val last_status: String = "",
    val next_run: String? = null,
)

@Serializable
data class TaskLog(
    val id: Long = 0,
    val task_id: Long = 0,
    val trigger: String = "",
    val status: String = "",
    val start_at: String = "",
    val end_at: String? = null,
    val cost_ms: Long = 0,
    val exit_code: Int = 0,
    val output: String = "",
)

// 进程管理
@Serializable
data class ProcInfo(val pid: Int = 0, val name: String = "", val cpu: Double = 0.0, val mem_pct: Double = 0.0)

@Serializable
data class ServiceInfo(
    val unit: String = "",
    val active: String = "",
    val sub: String = "",
    val enabled: String = "",
    val description: String = "",
)

// 防火墙
@Serializable
data class FirewallStatus(
    val tool: String = "",
    val enabled: Boolean = false,
    val anti_scan: Boolean = false,
    val message: String? = null,
)

@Serializable
data class FirewallRule(
    val id: Long = 0,
    val protocol: String = "",
    val ports: String = "",
    val source: String = "",
    val action: String = "",
    val direction: String = "",
    val remark: String = "",
    val auto: Boolean = false,
    val created_at: String = "",
)

// SSL 证书
@Serializable
data class CertInfo(
    val domain: String = "",
    val domains: List<String> = emptyList(),
    val not_after: String = "",
    val days_left: Int = 0,
    val source: String = "",
)

// Docker
@Serializable
data class DockerOverview(
    val installed: Boolean = false,
    val version: Map<String, kotlinx.serialization.json.JsonElement> = emptyMap(),
    val running: Int = 0,
    val stopped: Int = 0,
    val images: Int = 0,
)

@Serializable
data class DockerStatus(val installed: Boolean = false, val version: String = "", val compose: Boolean = false)

// 数据库
@Serializable
data class DbStatus(val mysql: DbEngineStatus = DbEngineStatus(), val pg: DbEngineStatus = DbEngineStatus())

@Serializable
data class DbEngineStatus(val installed: Boolean = false, val addr: String = "")

@Serializable
data class DbUser(val user: String = "", val host: String = "")

// 告警规则
@Serializable
data class AlertRule(
    val id: Long = 0,
    val name: String = "",
    val metric: String = "",
    val op: String = "",
    val threshold: Double = 0.0,
    val node_id: Long = 0,
    val duration_min: Int = 0,
    val cooldown_min: Int = 0,
    val enabled: Boolean = false,
    val last_fired_at: String? = null,
)

// 终端主机收藏（SSH 快连）
@Serializable
data class TermHost(
    val id: Long = 0,
    val name: String = "",
    val host: String = "",
    val port: Int = 22,
    val username: String = "",
    val auth_type: String = "password",
    val has_password: Boolean = false,
    val has_key: Boolean = false,
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


// ---- 1.3.0 面板组（用户管理/审计/通知与存储）与品牌 ----

@Serializable
data class UserItem(
    val id: Long = 0,
    val username: String = "",
    val role: String = "viewer",
    val disabled: Boolean = false,
    val created_at: String? = null,
)

@Serializable
data class AuditItem(
    val id: Long = 0,
    val username: String? = null,
    val role: String? = null,
    val action: String? = null,
    val method: String = "",
    val path: String = "",
    val ip: String? = null,
    val status: Int = 0,
    val cost_ms: Long = 0,
    val created_at: String? = null,
)

@Serializable
data class TermLogItem(
    val id: Long = 0,
    val username: String? = null,
    val line: String = "",
    val created_at: String? = null,
)

@Serializable
data class NotifyEmailCfg(
    var enabled: Boolean = false,
    var host: String = "",
    var port: Int = 465,
    var ssl: Boolean = true,
    var user: String = "",
    var pass: String = "",
    var from: String = "",
    var to: List<String> = emptyList(),
)

@Serializable
data class NotifyWebhookCfg(
    var enabled: Boolean = false,
    var type: String = "dingtalk",
    var url: String = "",
    var chat_id: String? = null,
)

@Serializable
data class S3Cfg(
    var enabled: Boolean = false,
    var endpoint: String = "",
    var access_key: String = "",
    var secret_key: String = "",
    var bucket: String = "",
    var use_ssl: Boolean = true,
    var prefix: String = "backups/",
)

@Serializable
data class BrandInfo(val name: String = "", val version: String = "")


@Serializable
data class NotifyConfigAll(val email: NotifyEmailCfg = NotifyEmailCfg(), val webhook: NotifyWebhookCfg = NotifyWebhookCfg())

@Serializable
data class AuditOpsPage(val items: List<AuditItem> = emptyList(), val total: Long = 0)

@Serializable
data class AuditTtyPage(val items: List<TermLogItem> = emptyList(), val total: Long = 0)
