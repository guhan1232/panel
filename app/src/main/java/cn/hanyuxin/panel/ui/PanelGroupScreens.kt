// PanelGroupScreens.kt —— 1.3.0 面板组三页：审计日志 / 通知与存储 / 用户管理。
package cn.hanyuxin.panel.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Tab
import androidx.compose.material3.TabRow
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.Api
import cn.hanyuxin.panel.data.AuditItem
import cn.hanyuxin.panel.data.NotifyEmailCfg
import cn.hanyuxin.panel.data.NotifyWebhookCfg
import cn.hanyuxin.panel.data.S3Cfg
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.TermLogItem
import cn.hanyuxin.panel.data.UserItem
import cn.hanyuxin.panel.data.clientFor
import kotlinx.coroutines.launch

// ---- 通用小件 ----

@Composable
private fun PageScaffold(title: String, content: @Composable () -> Unit) {
    Column(Modifier.fillMaxSize()) {
        Column(Modifier.padding(horizontal = 16.dp)) {
            Spacer(Modifier.height(4.dp))
            Text(title, style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)
            Spacer(Modifier.height(10.dp))
        }
        content()
    }
}

@Composable
internal fun StatusPill(text: String, tint: Color) {
    Text(
        text, fontSize = 10.sp, fontWeight = FontWeight.Medium, color = tint,
        modifier = Modifier
            .background(tint.copy(alpha = 0.14f), RoundedCornerShape(6.dp))
            .padding(horizontal = 6.dp, vertical = 2.dp),
    )
}

// ================= 审计日志 =================

@Composable
fun AuditScreen(server: ServerInfo) {
    val scope = rememberCoroutineScope()
    var tab by remember { mutableIntStateOf(0) }
    var items by remember { mutableStateOf<List<Any>>(emptyList()) }
    var total by remember { mutableIntStateOf(0) }
    var page by remember { mutableIntStateOf(1) }
    var loading by remember { mutableStateOf(true) }
    var err by remember { mutableStateOf<String?>(null) }
    var fMethod by remember { mutableStateOf("") }
    var fPath by remember { mutableStateOf("") }
    var fUser by remember { mutableStateOf("") }

    val client = clientFor(server)
    fun load() {
        loading = true; err = null
        scope.launch {
            apiGuard(onError = { err = it.message }) {
                if (tab == 0) {
                    val d = Api.auditOps(client, page, 20, fMethod, fPath, fUser)
                    items = d.items; total = d.total.toInt()
                } else {
                    val d = Api.auditTty(client, page, 20, fUser)
                    items = d.items; total = d.total.toInt()
                }
            }
            loading = false
        }
    }
    LaunchedEffect(tab, page) { load() }

    PageScaffold("审计日志") {
        TabRow(selectedTabIndex = tab) {
            Tab(tab == 0, onClick = { tab = 0; page = 1; items = emptyList() }, text = { Text("操作日志") })
            Tab(tab == 1, onClick = { tab = 1; page = 1; items = emptyList() }, text = { Text("终端命令") })
        }
        ErrorBanner(err)
        if (tab == 0) {
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
            ) {
                OutlinedTextField(
                    fMethod, { fMethod = it }, label = { Text("方法") },
                    modifier = Modifier.weight(0.9f), singleLine = true, textStyle = MaterialTheme.typography.bodySmall,
                )
                OutlinedTextField(
                    fPath, { fPath = it }, label = { Text("路径") },
                    modifier = Modifier.weight(1.6f), singleLine = true, textStyle = MaterialTheme.typography.bodySmall,
                )
            }
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 16.dp),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                OutlinedTextField(
                    fUser, { fUser = it }, label = { Text("操作人") },
                    modifier = Modifier.weight(1f), singleLine = true, textStyle = MaterialTheme.typography.bodySmall,
                )
                ActionChip("查询", busy = false) { page = 1; load() }
            }
        } else {
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                OutlinedTextField(
                    fUser, { fUser = it }, label = { Text("操作人") },
                    modifier = Modifier.weight(1f), singleLine = true, textStyle = MaterialTheme.typography.bodySmall,
                )
                ActionChip("查询", busy = false) { page = 1; load() }
            }
        }
        if (loading) {
            Box(Modifier.fillMaxWidth().padding(40.dp), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        } else {
            LazyColumn(
                Modifier.fillMaxSize(),
                contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 16.dp, vertical = 8.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                item {
                    Text(
                        "共 $total 条 · 第 $page 页",
                        fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                if (items.isEmpty()) item { EmptyView("暂无记录") }
                if (tab == 0) {
                    items(items, key = { (it as AuditItem).id }) { raw ->
                        val it = raw as AuditItem
                        PanelCard {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Text(
                                    "${it.method} ${it.status}",
                                    fontSize = 12.sp, fontWeight = FontWeight.SemiBold,
                                    color = if (it.method == "DELETE") MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary,
                                    modifier = Modifier.weight(1f),
                                )
                                Text(it.created_at?.take(19)?.replace('T', ' ') ?: "", fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                            Text(it.path, fontSize = 12.sp, fontFamily = FontFamily.Monospace, maxLines = 1)
                            Text(
                                listOfNotNull(
                                    it.action?.takeIf { a -> a.isNotBlank() && a != it.method + " " + it.path },
                                    it.username, it.ip,
                                    it.cost_ms.takeIf { c -> c > 0 }?.let { c -> "${c}ms" },
                                ).joinToString(" · "),
                                fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                } else {
                    items(items, key = { (it as TermLogItem).id }) { raw ->
                        val it = raw as TermLogItem
                        PanelCard {
                            Text(
                                it.line, fontSize = 12.sp, fontFamily = FontFamily.Monospace,
                                color = MaterialTheme.colorScheme.onSurface, maxLines = 3,
                            )
                            Text(
                                listOfNotNull(it.username, it.created_at?.take(19)?.replace('T', ' ')).joinToString(" · "),
                                fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                    }
                }
                if (total > 20) {
                    item {
                        Row(
                            Modifier.fillMaxWidth().padding(vertical = 8.dp),
                            horizontalArrangement = Arrangement.Center,
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            TextButton(enabled = page > 1, onClick = { page-- }) { Text("上一页") }
                            Text("$page / ${(total + 19) / 20}", fontSize = 12.sp)
                            TextButton(enabled = page * 20 < total, onClick = { page++ }) { Text("下一页") }
                        }
                    }
                }
            }
        }
    }
}

// ================= 通知与存储 =================

@Composable
fun NotifyScreen(server: ServerInfo) {
    val scope = rememberCoroutineScope()
    var email by remember { mutableStateOf(NotifyEmailCfg()) }
    var toStr by remember { mutableStateOf("") }
    var hook by remember { mutableStateOf(NotifyWebhookCfg()) }
    var s3 by remember { mutableStateOf(S3Cfg()) }
    var loading by remember { mutableStateOf(true) }
    var toast by remember { mutableStateOf<String?>(null) }

    val client = clientFor(server)
    fun toast(s: String) { toast = s }
    LaunchedEffect(Unit) {
        apiGuard(onError = { toast = it.message }) {
            val cfg = Api.notifyConfig(client)
            email = cfg.email
            toStr = cfg.email.to.joinToString(",")
            hook = cfg.webhook
            s3 = Api.s3Config(client)
        }
        loading = false
    }

    PageScaffold("通知与存储") {
        if (loading) {
            Box(Modifier.fillMaxWidth().padding(40.dp), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
            return@PageScaffold
        }
        LazyColumn(
            Modifier.fillMaxSize(),
            contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 16.dp, vertical = 8.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            toast?.let { item { Text(it, color = MaterialTheme.colorScheme.primary, fontSize = 12.sp) } }

            // ---- SMTP ----
            item {
                PanelCard {
                    Text("邮件通知 (SMTP)", fontWeight = FontWeight.SemiBold)
                    Spacer(Modifier.height(8.dp))
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Switch(email.enabled, { email = email.copy(enabled = it) })
                        Spacer(Modifier.width(8.dp))
                        Text("启用", fontSize = 13.sp)
                    }
                    LabeledField("SMTP 服务器", email.host) { email = email.copy(host = it) }
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        LabeledField("端口", email.port.toString(), modifier = Modifier.weight(1f)) { email.port = it.toIntOrNull() ?: 465 }
                        Column {
                            Text("SSL", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            Switch(email.ssl, { email = email.copy(ssl = it) })
                        }
                    }
                    LabeledField("账号", email.user) { email = email.copy(user = it) }
                    LabeledField("密码/授权码", email.pass, isPassword = true) { email = email.copy(pass = it) }
                    LabeledField("发件人", email.from) { email = email.copy(from = it) }
                    LabeledField("收件人（逗号分隔）", toStr) { toStr = it }
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        ActionChip("发送测试邮件", busy = false) {
                            scope.launch {
                                var ok = false
                                apiGuard(onError = { toast = it.message }) { Api.emailSave(client, email, toStr.split(",").map { x -> x.trim() }.filter { x -> x.isNotEmpty() }); Api.notifyTest(client, "email"); ok = true }
                                if (ok) toast = "已发送，请查收"
                            }
                        }
                        ActionChip("保存", busy = false) { scope.launch { var ok = false; apiGuard(onError = { toast = it.message }) { Api.emailSave(client, email, toStr.split(",").map { x -> x.trim() }.filter { x -> x.isNotEmpty() }); ok = true }; if (ok) toast = "已保存" } }
                    }
                }
            }

            // ---- IM Webhook ----
            item {
                PanelCard {
                    Text("IM Webhook 通知", fontWeight = FontWeight.SemiBold)
                    Spacer(Modifier.height(8.dp))
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Switch(hook.enabled, { hook = hook.copy(enabled = it) })
                        Spacer(Modifier.width(8.dp))
                        Text("启用", fontSize = 13.sp)
                    }
                    LabeledField("类型 (dingtalk/wecom/feishu/generic/serverchan/bark/telegram)", hook.type) { hook = hook.copy(type = it) }
                    LabeledField("Webhook URL", hook.url) { hook = hook.copy(url = it) }
                    if (hook.type == "telegram") LabeledField("Chat ID", hook.chat_id ?: "") { hook = hook.copy(chat_id = it) }
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        ActionChip("发送测试消息", busy = false) {
                            scope.launch {
                                var ok = false
                                apiGuard(onError = { toast = it.message }) { Api.webhookSave(client, hook); Api.notifyTest(client, "webhook"); ok = true }
                                if (ok) toast = "已发送"
                            }
                        }
                        ActionChip("保存", busy = false) { scope.launch { var ok = false; apiGuard(onError = { toast = it.message }) { Api.webhookSave(client, hook); ok = true }; if (ok) toast = "已保存" } }
                    }
                }
            }

            // ---- S3 ----
            item {
                PanelCard {
                    Text("S3 兼容对象存储", fontWeight = FontWeight.SemiBold)
                    Spacer(Modifier.height(8.dp))
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Switch(s3.enabled, { s3 = s3.copy(enabled = it) })
                        Spacer(Modifier.width(8.dp))
                        Text("启用", fontSize = 13.sp)
                    }
                    LabeledField("Endpoint", s3.endpoint) { s3 = s3.copy(endpoint = it) }
                    LabeledField("AccessKey", s3.access_key) { s3 = s3.copy(access_key = it) }
                    LabeledField("SecretKey", s3.secret_key, isPassword = true) { s3 = s3.copy(secret_key = it) }
                    LabeledField("桶名", s3.bucket) { s3 = s3.copy(bucket = it) }
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Switch(s3.use_ssl, { s3 = s3.copy(use_ssl = it) })
                        Spacer(Modifier.width(8.dp))
                        Text("HTTPS", fontSize = 13.sp)
                    }
                    LabeledField("前缀", s3.prefix) { s3 = s3.copy(prefix = it) }
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        ActionChip("测试连通性", busy = false) {
                            scope.launch {
                                var ok = false
                                apiGuard(onError = { toast = it.message }) { Api.s3Save(client, s3); Api.s3Test(client); ok = true }
                                if (ok) toast = "桶连接正常"
                            }
                        }
                        ActionChip("保存", busy = false) { scope.launch { var ok = false; apiGuard(onError = { toast = it.message }) { Api.s3Save(client, s3); ok = true }; if (ok) toast = "已保存" } }
                    }
                }
            }
        }
    }
}

@Composable
private fun LabeledField(
    label: String,
    value: String,
    modifier: Modifier = Modifier.fillMaxWidth(),
    isPassword: Boolean = false,
    onValue: (String) -> Unit,
) {
    Column(modifier.padding(vertical = 4.dp)) {
        Text(label, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        OutlinedTextField(
            value, onValue,
            modifier = Modifier.fillMaxWidth(),
            singleLine = !isPassword || true,
            visualTransformation = if (isPassword) PasswordVisualTransformation() else androidx.compose.ui.text.input.VisualTransformation.None,
            textStyle = MaterialTheme.typography.bodySmall,
        )
    }
}

// ================= 用户管理 =================

private val ROLE_LABEL = mapOf("admin" to "管理员", "operator" to "操作员", "viewer" to "只读")

@Composable
fun UsersScreen(server: ServerInfo) {
    val scope = rememberCoroutineScope()
    var users by remember { mutableStateOf<List<UserItem>?>(null) }
    var busyId by remember { mutableStateOf<Long?>(null) }
    var toast by remember { mutableStateOf<String?>(null) }
    var createOpen by remember { mutableStateOf(false) }
    var editTarget by remember { mutableStateOf<UserItem?>(null) }
    var delTarget by remember { mutableStateOf<UserItem?>(null) }

    val client = clientFor(server)
    fun load() {
        scope.launch { apiGuard(onError = { toast = it.message }) { users = Api.userList(client) } }
    }
    LaunchedEffect(Unit) { load() }

    PageScaffold("用户管理") {
        Row(
            Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                "三角角色 RBAC · 禁止修改自身角色/删除自身",
                fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f),
            )
            IconButton(onClick = { createOpen = true }) { Icon(Icons.Outlined.Add, "新建用户", tint = MaterialTheme.colorScheme.primary) }
        }
        ErrorBanner(toast)
        val list = users
        if (list == null) {
            Box(Modifier.fillMaxWidth().padding(40.dp), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
        } else {
            LazyColumn(
                Modifier.fillMaxSize(),
                contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 16.dp, vertical = 8.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                items(list, key = { it.id }) { u ->
                    PanelCard {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text(u.username, fontWeight = FontWeight.SemiBold)
                                Text(
                                    ROLE_LABEL[u.role] ?: u.role,
                                    fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                            if (u.disabled) StatusPill("已禁用", IdleGray) else StatusPill("正常", OkGreen)
                            Spacer(Modifier.width(6.dp))
                            IconButton(onClick = { editTarget = u }) { Icon(Icons.Outlined.Edit, "编辑", tint = MaterialTheme.colorScheme.primary) }
                            IconButton(onClick = { delTarget = u }) { Icon(Icons.Outlined.Delete, "删除", tint = MaterialTheme.colorScheme.error) }
                        }
                    }
                }
                if (list.isEmpty()) item { EmptyView("还没有用户") }
            }
        }
    }

    // 新建/编辑对话框
    if (createOpen || editTarget != null) {
        val editing = editTarget
        var username by remember { mutableStateOf(editing?.username ?: "") }
        var password by remember { mutableStateOf("") }
        var role by remember { mutableStateOf(editing?.role ?: "operator") }
        var disabled by remember { mutableStateOf(editing?.disabled ?: false) }
        AlertDialog(
            onDismissRequest = { createOpen = false; editTarget = null },
            title = { Text(if (editing != null) "编辑 ${editing.username}" else "新建用户") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (editing == null) OutlinedTextField(username, { username = it }, label = { Text("用户名") }, singleLine = true)
                    OutlinedTextField(
                        password, { password = it },
                        label = { Text(if (editing != null) "重置密码（留空不修改）" else "密码") },
                        singleLine = true,
                        visualTransformation = PasswordVisualTransformation(),
                    )
                    Text("角色", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        FilterChip(role == "admin", { role = "admin" }, label = { Text("管理员") })
                        FilterChip(role == "operator", { role = "operator" }, label = { Text("操作员") })
                        FilterChip(role == "viewer", { role = "viewer" }, label = { Text("只读") })
                    }
                    if (editing != null) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Switch(disabled, { disabled = it })
                            Spacer(Modifier.width(8.dp))
                            Text(if (disabled) "已禁用登录" else "正常登录", fontSize = 13.sp)
                        }
                    }
                }
            },
            confirmButton = {
                TextButton(onClick = {
                    scope.launch {
                        var ok = false
                        apiGuard(onError = { toast = it.message }) {
                            if (editing != null) {
                                Api.userUpdate(client, editing.id, role.takeIf { it != editing.role }, password.takeIf { it.isNotBlank() }, disabled.takeIf { it != editing.disabled })
                            } else {
                                Api.userCreate(client, username.trim(), password, role)
                            }
                            ok = true
                        }
                        if (ok) {
                            toast = if (editing != null) "已保存" else "已创建"
                            createOpen = false; editTarget = null
                            load()
                        }
                    }
                }) { Text(if (editing != null) "保存" else "创建") }
            },
            dismissButton = { TextButton(onClick = { createOpen = false; editTarget = null }) { Text("取消") } },
        )
    }

    // 删除确认
    delTarget?.let { u ->
        AlertDialog(
            onDismissRequest = { delTarget = null },
            title = { Text("删除用户 ${u.username}") },
            text = { Text("确认删除该用户？该操作不可恢复。") },
            confirmButton = {
                TextButton(onClick = {
                    busyId = u.id; delTarget = null
                    scope.launch {
                        var ok = false
                        apiGuard(onError = { toast = it.message }) { Api.userDelete(client, u.id); ok = true }
                        if (ok) toast = "已删除"
                        busyId = null; load()
                    }
                }) { Text("删除", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { delTarget = null }) { Text("取消") } },
        )
    }
}
