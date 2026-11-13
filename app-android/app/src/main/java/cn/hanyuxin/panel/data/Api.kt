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

    // ---- 绑定（docs/APP.md）：绑定密钥免密登录 ----

    /** 绑定登录：绑定密钥换取 30 天会话（免账号密码），成功后自动保存 token。 */
    suspend fun bindLogin(server: ServerInfo, bindKey: String, deviceName: String): LoginResp {
        val anon = PanelClient(server, null)
        val body = Http.json.encodeToString(
            mapOf("bind_key" to bindKey, "device_name" to deviceName),
        )
        val resp = anon.request<LoginResp>("POST", "/api/auth/bind/login", body)
            ?: throw ApiException(-1, "绑定登录响应为空")
        if (resp.token.isNullOrBlank()) throw ApiException(-1, "绑定成功但未返回会话令牌")
        ServerStore.setToken(server.host, resp.token)
        ServerStore.upsert(server.host, server.entrance, server.name)
        ServerStore.setLastHost(server.host)
        Http.invalidate(server.host)
        return resp
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
        return anon.request<UpdateInfo>("GET", "/api/app/check-update", query = listOf("platform" to "android", "v" to BuildVersion.CURRENT)) ?: UpdateInfo()
    }

    // ---- Docker（/api/apps/docker/*）----

    suspend fun dockerOverview(c: PanelClient): DockerOverview =
        c.request<DockerOverview>("GET", "/api/apps/docker/overview") ?: DockerOverview()

    /** 容器列表：docker ps -a 原生 JSON（ID/Names/Image/State/Status/Ports 大写字段）。 */
    suspend fun dockerContainers(c: PanelClient): List<Map<String, kotlinx.serialization.json.JsonElement>> =
        c.request("GET", "/api/apps/docker/containers") ?: emptyList()

    suspend fun dockerContainerAction(c: PanelClient, name: String, action: String, force: Boolean = false) {
        val q = if (force) listOf("force" to "1") else emptyList()
        c.request<Unit>("POST", "/api/apps/docker/containers/$name/$action", "{}", query = q)
    }

    suspend fun dockerLogs(c: PanelClient, name: String, tail: Int = 200): String =
        c.request<Map<String, String>>("GET", "/api/apps/docker/containers/$name/logs", query = listOf("tail" to tail.toString()))?.get("logs") ?: ""

    suspend fun dockerImages(c: PanelClient): List<Map<String, kotlinx.serialization.json.JsonElement>> =
        c.request("GET", "/api/apps/docker/images") ?: emptyList()

    suspend fun dockerPull(c: PanelClient, image: String): Pair<Boolean, String> {
        val d = c.request<Map<String, kotlinx.serialization.json.JsonElement>>("POST", "/api/apps/docker/pull",
            Http.json.encodeToString(mapOf("image" to image))) ?: return false to "无响应"
        val ok = (d["ok"] as? kotlinx.serialization.json.JsonPrimitive)?.content == "true"
        val out = (d["output"] as? kotlinx.serialization.json.JsonPrimitive)?.content ?: ""
        return ok to out
    }

    suspend fun dockerPrune(c: PanelClient): String {
        val d = c.request<Map<String, kotlinx.serialization.json.JsonElement>>("POST", "/api/apps/docker/images/prune", "{}", query = listOf("force" to "1"))
        return (d?.get("output") as? kotlinx.serialization.json.JsonPrimitive)?.content ?: ""
    }

    // ---- 数据库（/api/dbmgr/*）----

    suspend fun dbStatus(c: PanelClient): DbStatus = c.request<DbStatus>("GET", "/api/dbmgr/status") ?: DbStatus()
    suspend fun dbDatabases(c: PanelClient, engine: String): List<String> =
        c.request<List<String>>("GET", "/api/dbmgr/$engine/databases") ?: emptyList()
    suspend fun dbCreateDatabase(c: PanelClient, engine: String, name: String) {
        c.request<Unit>("POST", "/api/dbmgr/$engine/databases", Http.json.encodeToString(mapOf("name" to name)))
    }
    suspend fun dbDeleteDatabase(c: PanelClient, engine: String, name: String) {
        c.request<Unit>("DELETE", "/api/dbmgr/$engine/databases/$name")
    }
    suspend fun dbUsers(c: PanelClient, engine: String): List<Any> =
        if (engine == "mysql") c.request<List<DbUser>>("GET", "/api/dbmgr/mysql/users") ?: emptyList()
        else c.request<List<String>>("GET", "/api/dbmgr/pg/users") ?: emptyList()
    suspend fun dbCreateUser(c: PanelClient, engine: String, name: String, host: String, password: String) {
        val body = if (engine == "mysql") mapOf("name" to name, "host" to host, "password" to password)
        else mapOf("name" to name, "password" to password)
        c.request<Unit>("POST", "/api/dbmgr/$engine/users", Http.json.encodeToString(body))
    }
    suspend fun dbGrant(c: PanelClient, engine: String, user: String, host: String, db: String) {
        val body = if (engine == "mysql") mapOf("user" to user, "host" to host, "db" to db)
        else mapOf("user" to user, "db" to db)
        c.request<Unit>("POST", "/api/dbmgr/$engine/grant", Http.json.encodeToString(body))
    }
    suspend fun dbDeleteUserMysql(c: PanelClient, userAtHost: String) {
        val user = userAtHost.substringBefore("@")
        val host = userAtHost.substringAfter("@", "%")
        c.request<Unit>("DELETE", "/api/dbmgr/mysql/users/$user/$host")
    }
    suspend fun dbDeleteUserPg(c: PanelClient, name: String) {
        c.request<Unit>("DELETE", "/api/dbmgr/pg/users/$name")
    }

    // ---- 计划任务（/api/tasks）----

    suspend fun tasks(c: PanelClient): List<Task> = c.request<List<Task>>("GET", "/api/tasks") ?: emptyList()
    suspend fun taskToggle(c: PanelClient, id: Long) { c.request<Unit>("POST", "/api/tasks/$id/toggle", "{}") }
    suspend fun taskRun(c: PanelClient, id: Long) { c.request<Unit>("POST", "/api/tasks/$id/run", "{}") }
    suspend fun taskDelete(c: PanelClient, id: Long) { c.request<Unit>("DELETE", "/api/tasks/$id") }
    suspend fun taskCreate(c: PanelClient, name: String, spec: String, command: String): Task? {
        val body = Http.json.encodeToString(mapOf("name" to name, "type" to "shell", "spec" to spec, "command" to command))
        return c.request<Task>("POST", "/api/tasks", body)
    }
    suspend fun taskLogs(c: PanelClient, id: Long, page: Int = 1, size: Int = 20): PageResp<TaskLog> =
        c.request<PageResp<TaskLog>>("GET", "/api/tasks/$id/logs", query = listOf("page" to page.toString(), "size" to size.toString()))
            ?: PageResp()
    suspend fun taskLogDetail(c: PanelClient, logId: Long): TaskLog? =
        c.request<TaskLog>("GET", "/api/task-logs/$logId")

    // ---- 进程管理（/api/proc）----

    suspend fun procList(c: PanelClient, sort: String = "cpu"): List<ProcInfo> =
        c.request<List<ProcInfo>>("GET", "/api/proc/list", query = listOf("sort" to sort)) ?: emptyList()
    suspend fun procKill(c: PanelClient, pid: Int, force: Boolean) {
        c.request<Unit>("POST", "/api/proc/kill", Http.json.encodeToString(mapOf("pid" to pid, "force" to force)))
    }
    suspend fun procServices(c: PanelClient): List<ServiceInfo> = c.request<List<ServiceInfo>>("GET", "/api/proc/services") ?: emptyList()
    suspend fun procServiceAction(c: PanelClient, unit: String, action: String) {
        c.request<Unit>("POST", "/api/proc/service", Http.json.encodeToString(mapOf("name" to unit, "action" to action)))
    }

    // ---- 节点（/api/nodes）----

    suspend fun nodeExec(c: PanelClient, id: Long, script: String): String {
        val d = c.request<Map<String, kotlinx.serialization.json.JsonElement>>("POST", "/api/nodes/$id/exec",
            Http.json.encodeToString(mapOf("script" to script))) ?: return "无响应"
        return (d["output"] as? kotlinx.serialization.json.JsonPrimitive)?.content ?: ""
    }

    // ---- 防火墙（/api/firewall）----

    suspend fun fwStatus(c: PanelClient): FirewallStatus = c.request<FirewallStatus>("GET", "/api/firewall/status") ?: FirewallStatus()
    suspend fun fwRules(c: PanelClient): List<FirewallRule> = c.request<List<FirewallRule>>("GET", "/api/firewall/rules") ?: emptyList()
    suspend fun fwAddRule(c: PanelClient, protocol: String, ports: String, action: String, source: String, remark: String): FirewallRule? {
        val body = Http.json.encodeToString(mapOf(
            "protocol" to protocol, "ports" to ports, "action" to action,
            "direction" to "in", "source" to source, "remark" to remark,
        ))
        return c.request<FirewallRule>("POST", "/api/firewall/rule", body)
    }
    suspend fun fwDeleteRule(c: PanelClient, id: Long) { c.request<Unit>("DELETE", "/api/firewall/rule/$id") }
    suspend fun fwSync(c: PanelClient) { c.request<Unit>("POST", "/api/firewall/sync", "{}") }
    suspend fun fwAntiScan(c: PanelClient, enabled: Boolean) {
        c.request<Unit>("POST", "/api/firewall/anti-scan", Http.json.encodeToString(mapOf("enabled" to enabled)))
    }

    // ---- SSL 证书（/api/certs）----

    suspend fun certs(c: PanelClient): List<CertInfo> = c.request<List<CertInfo>>("GET", "/api/certs") ?: emptyList()
    suspend fun certDelete(c: PanelClient, domain: String) { c.request<Unit>("DELETE", "/api/certs/$domain") }
    suspend fun certUpload(c: PanelClient, certPem: String, keyPem: String) {
        c.request<Unit>("POST", "/api/certs/upload", Http.json.encodeToString(mapOf("cert_pem" to certPem, "key_pem" to keyPem)))
    }
    suspend fun certIssue(c: PanelClient, domains: List<String>, email: String): Map<String, String> {
        val body = Http.json.encodeToString(mapOf(
            "domains" to domains, "email" to email, "challenge" to "http01",
        ))
        val d = c.request<Map<String, String>>("POST", "/api/certs/issue", body) ?: emptyMap()
        return d
    }

    // ---- 告警规则（/api/alerts）----

    suspend fun alertRules(c: PanelClient): List<AlertRule> = c.request<List<AlertRule>>("GET", "/api/alerts/rules") ?: emptyList()
    suspend fun alertRuleToggle(c: PanelClient, id: Long) { c.request<Unit>("POST", "/api/alerts/rules/$id/toggle", "{}") }
    suspend fun alertRuleDelete(c: PanelClient, id: Long) { c.request<Unit>("DELETE", "/api/alerts/rules/$id") }
    suspend fun alertRuleCreate(c: PanelClient, name: String, metric: String, op: String, threshold: Double): AlertRule? {
        val body = Http.json.encodeToString(mapOf(
            "name" to name, "metric" to metric, "op" to op, "threshold" to threshold,
            "duration_min" to 1, "cooldown_min" to 10, "enabled" to true,
        ))
        return c.request<AlertRule>("POST", "/api/alerts/rules", body)
    }

    // ---- 终端主机收藏 ----

    suspend fun termHosts(c: PanelClient): List<TermHost> = c.request<List<TermHost>>("GET", "/api/term/hosts") ?: emptyList()
}

/** APP 当前版本号（与 build.gradle.kts versionName 同步）。 */
object BuildVersion { const val CURRENT = "1.1.0" }
