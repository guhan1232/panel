// MeScreens.kt —— 「我的」（系统设置：服务器信息/设备管理/生物识别锁/检查更新/退出）与消息中心。
package cn.hanyuxin.panel.ui

import android.app.DownloadManager
import android.content.Context
import android.content.Intent
import android.os.Environment
import android.net.Uri
import androidx.biometric.BiometricManager
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.TabRow
import androidx.compose.material3.Tab
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.FilterChip
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.Devices
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material.icons.outlined.Fingerprint
import androidx.compose.material.icons.outlined.Logout
import androidx.compose.material.icons.outlined.Notifications
import androidx.compose.material.icons.outlined.QrCodeScanner
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material.icons.outlined.SwapHoriz
import androidx.compose.material.icons.outlined.SystemUpdate
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.Api
import cn.hanyuxin.panel.data.PanelInfo
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.ServerStore
import cn.hanyuxin.panel.data.SessionItem
import cn.hanyuxin.panel.data.UpdateInfo
import cn.hanyuxin.panel.data.clientFor
import java.io.File
import java.security.MessageDigest
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

// 语义色（与 Docker 页状态色共用一套）。
internal val OkGreen = Color(0xFF22C55E)
internal val IdleGray = Color(0xFF9CA3AF)
internal val SkyBlue = Color(0xFF38BDF8)
internal val Amber = Color(0xFFF59E0B)
internal val Violet = Color(0xFFA78BFA)
internal val Emerald = Color(0xFF34D399)
internal val Slate = Color(0xFF94A3B8)

@Composable
fun MeScreen(server: ServerInfo) {
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()
    var info by remember(server.host) { mutableStateOf<PanelInfo?>(null) }
    var bio by remember { mutableStateOf(ServerStore.bioEnabled()) }
    var showDevices by remember { mutableStateOf(false) }
    var update by remember { mutableStateOf<UpdateInfo?>(null) }
    var dlProgress by remember { mutableIntStateOf(-1) } // -1 未下载；0-99 下载中；100 校验中
    var dlId by remember { mutableStateOf<Long?>(null) }
    var checking by remember { mutableStateOf(false) }
    var panelName by remember { mutableStateOf("") }

    LaunchedEffect(server.host) {
        apiGuard { Api.panelInfo(clientFor(server)) }?.let { info = it }
        apiGuard { Api.brandInfo(clientFor(server)) }?.let { panelName = it.name }
    }

    fun toast(s: String) = android.widget.Toast.makeText(ctx, s, android.widget.Toast.LENGTH_SHORT).show()

    fun toggleBio() {
        val bm = BiometricManager.from(ctx)
        val can = bm.canAuthenticate(BiometricManager.Authenticators.BIOMETRIC_WEAK or BiometricManager.Authenticators.DEVICE_CREDENTIAL)
        if (can != BiometricManager.BIOMETRIC_SUCCESS) {
            toast("设备未设置锁屏密码或指纹，无法开启")
        } else {
            bio = !bio
            ServerStore.setBioEnabled(bio)
            toast(if (bio) "生物识别锁已开启" else "生物识别锁已关闭")
        }
    }

    LazyColumn(
        Modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        // 品牌渐变头卡：当前面板 + 版本/系统/协议徽标
        item {
            Box(
                Modifier
                    .fillMaxWidth()
                    .background(PanelBrandBrush, RoundedCornerShape(20.dp))
                    .padding(18.dp),
            ) {
                Column {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(
                            Modifier.size(44.dp).background(Color.White.copy(alpha = 0.18f), RoundedCornerShape(13.dp)),
                            contentAlignment = Alignment.Center,
                        ) {
                            Icon(Icons.Outlined.Dns, null, tint = Color.White, modifier = Modifier.size(24.dp))
                        }
                        Spacer(Modifier.width(12.dp))
                        Column {
                            Text(
                                server.name.ifBlank { server.host },
                                color = Color.White, fontWeight = FontWeight.Bold, fontSize = 16.sp,
                            )
                            Text(
                                server.host,
                                color = Color.White.copy(alpha = 0.8f), fontSize = 11.sp, fontFamily = FontFamily.Monospace,
                            )
                        }
                    }
                    Spacer(Modifier.height(12.dp))
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        HeroChip("v${info?.version?.ifBlank { null } ?: "…"}")
                        HeroChip(info?.let { "${it.os}/${it.arch}" } ?: "服务端")
                        HeroChip(if (info?.tls == true) "HTTPS" else "HTTP")
                    }
                }
            }
        }

        // 安全与设备
        item {
            SectionLabel("安全与设备")
            GroupCard {
                SettingsRow(
                    Icons.Outlined.Fingerprint, Emerald,
                    "生物识别锁", "每次打开 APP 需指纹或锁屏凭据解锁",
                    chevron = false,
                    trailing = { Switch(checked = bio, onCheckedChange = { toggleBio() }) },
                    onClick = { toggleBio() },
                )
                GroupDivider()
                SettingsRow(Icons.Outlined.Devices, SkyBlue, "登录设备", "查看与踢出在线会话") { showDevices = true }
            }
        }

        // 通用
        item {
            SectionLabel("通用")
                GroupDivider()
                SettingsRow(Icons.Outlined.Settings, Slate, "系统设置", "面板名称 / 密码 / API Token") {
                    Router.push(Router.Route.Settings(server))
                }
            GroupCard {
                SettingsRow(Icons.Outlined.Notifications, Amber, "消息中心", "告警规则触发事件") {
                    Router.push(Router.Route.Alerts(server) { Router.pop() })
                }
                GroupDivider()
                SettingsRow(
                    Icons.Outlined.SystemUpdate, Violet,
                    "检查更新", "APP 自更新，保持与面板版本配套",
                    chevron = !checking,
                    trailing = if (checking) {
                        { CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp) }
                    } else null,
                ) {
                    checking = true
                    scope.launch {
                        apiGuard(onError = { toast(it.message ?: "检查失败") }) { Api.checkUpdate(server) }?.let { update = it }
                        checking = false
                    }
                }
                GroupDivider()
                SettingsRow(Icons.Outlined.QrCodeScanner, OkGreen, "扫码添加服务器", "扫 Web 登录页二维码") {
                    Router.push(Router.Route.Scanner(onQr = { handleScan(it) }, onClose = { Router.pop() }))
                }
            }
        }

        // 服务器
        item {
            SectionLabel("服务器")
            GroupCard {
                SettingsRow(Icons.Outlined.SwapHoriz, SkyBlue, "切换服务器", "返回服务器列表，可免密切回") { Session.leave() }
            }
        }

        // 危险区：退出登录
        item {
            OutlinedButton(
                onClick = {
                    scope.launch {
                        apiGuard { Api.logout(clientFor(server), server.host) }
                        Session.leave(clearToken = true)
                        toast("已退出登录")
                    }
                },
                modifier = Modifier.fillMaxWidth().height(46.dp),
                shape = RoundedCornerShape(13.dp),
                colors = ButtonDefaults.outlinedButtonColors(contentColor = MaterialTheme.colorScheme.error),
            ) {
                Icon(Icons.Outlined.Logout, null, modifier = Modifier.size(17.dp))
                Spacer(Modifier.width(6.dp))
                Text("退出登录", fontSize = 14.sp, fontWeight = FontWeight.Medium)
            }
        }

        item {
            Text(
                (panelName.ifBlank { "寒语馨面板" }) + " APP · Kotlin / Compose",
                fontSize = 11.sp,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.fillMaxWidth().padding(top = 2.dp),
                textAlign = androidx.compose.ui.text.style.TextAlign.Center,
            )
        }
    }

    if (showDevices) {
        DevicesDialog(server) { showDevices = false }
    }

    update?.let { u ->
        AlertDialog(
            onDismissRequest = { update = null },
            title = { Text(if (u.has_update) "发现新版本 ${u.latest}" else "已是最新版本") },
            text = {
                Column {
                    Text("当前版本：${u.current.ifBlank { info?.version ?: "" }}", fontSize = 13.sp)
                    if (u.notes.isNotBlank()) {
                        Spacer(Modifier.height(6.dp))
                        Text(u.notes, fontSize = 13.sp)
                    }
                }
            },
            confirmButton = {
                when {
                    dlProgress >= 0 && dlProgress < 100 -> {
                        Column(horizontalAlignment = Alignment.CenterHorizontally) {
                            androidx.compose.material3.LinearProgressIndicator(progress = dlProgress / 100f, modifier = Modifier.fillMaxWidth(0.6f))
                            Spacer(Modifier.height(6.dp))
                            Text("下载中 $dlProgress%", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                    dlProgress == 100 -> Text("校验中…", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    u.has_update && u.apk_url.isNotBlank() -> TextButton(onClick = {
                        val dm = ctx.getSystemService(Context.DOWNLOAD_SERVICE) as DownloadManager
                        val req = DownloadManager.Request(Uri.parse(u.apk_url))
                            .setTitle("寒语馨面板 " + u.latest)
                            .setDescription("正在下载更新包")
                            .setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED)
                            .setDestinationInExternalFilesDir(ctx, Environment.DIRECTORY_DOWNLOADS, "hanyuxin-panel-update.apk")
                            .setMimeType("application/vnd.android.package-archive")
                        dlProgress = 0
                        dlId = dm.enqueue(req)
                    }) { Text("下载并安装") }
                    else -> TextButton(onClick = { update = null }) { Text("知道了") }
                }
            },
            dismissButton = if (u.has_update) ({ TextButton(onClick = { update = null }) { Text("以后再说") } }) else null,
        )
    }
    // 更新包下载轮询：完成后 SHA256 校验（面板下发），通过则唤起系统安装器
    LaunchedEffect(dlId) {
        val id = dlId ?: return@LaunchedEffect
        val dm = ctx.getSystemService(Context.DOWNLOAD_SERVICE) as DownloadManager
        while (true) {
            val cur = dlId ?: break
            val c = dm.query(DownloadManager.Query().setFilterById(cur))
            var status = -1
            if (c.moveToFirst()) status = c.getInt(c.getColumnIndexOrThrow(DownloadManager.COLUMN_STATUS))
            c.close()
            when (status) {
                DownloadManager.STATUS_SUCCESSFUL -> {
                    dlProgress = 100
                    val uri = dm.getUriForDownloadedFile(cur)
                    val got = uri?.let { u2 -> ctx.contentResolver.openInputStream(u2)?.use { sha256Of(it) } } ?: ""
                    if (update?.sha256.isNullOrBlank() || got.equals(update?.sha256, ignoreCase = true)) {
                        dlProgress = -1; dlId = null; update = null
                        ctx.startActivity(Intent(Intent.ACTION_VIEW).apply {
                            setDataAndType(uri, "application/vnd.android.package-archive")
                            addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
                        })
                    } else {
                        dm.remove(cur); dlProgress = -1; dlId = null
                        toast("更新包校验失败（SHA256 不符），已取消安装")
                    }
                    break
                }
                DownloadManager.STATUS_FAILED -> {
                    dm.remove(cur); dlProgress = -1; dlId = null
                    toast("下载失败，请重试")
                    break
                }
                else -> {
                    val c2 = dm.query(DownloadManager.Query().setFilterById(cur))
                    if (c2.moveToFirst()) {
                        val bytes = c2.getLong(c2.getColumnIndexOrThrow(DownloadManager.COLUMN_BYTES_DOWNLOADED_SO_FAR))
                        val total = c2.getLong(c2.getColumnIndexOrThrow(DownloadManager.COLUMN_TOTAL_SIZE_BYTES))
                        if (total > 0) dlProgress = ((bytes * 100) / total).toInt().coerceIn(0, 99)
                    }
                    c2.close()
                    delay(500)
                }
            }
        }
    }
}

/** 头卡上的半透明信息胶囊。 */
@Composable
private fun HeroChip(text: String) {
    Text(
        text,
        fontSize = 10.sp,
        fontWeight = FontWeight.SemiBold,
        color = Color.White,
        modifier = Modifier
            .background(Color.White.copy(alpha = 0.18f), RoundedCornerShape(7.dp))
            .padding(horizontal = 8.dp, vertical = 3.dp),
    )
}

@Composable
private fun DevicesDialog(server: ServerInfo, onDismiss: () -> Unit) {
    val scope = rememberCoroutineScope()
    var sessions by remember { mutableStateOf<List<SessionItem>?>(null) }
    var reload by remember { mutableStateOf(0) }

    LaunchedEffect(server.host, reload) {
        apiGuard { Api.sessions(clientFor(server)) }?.let { sessions = it }
    }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("登录设备") },
        text = {
            val list = sessions
            when {
                list == null -> Row { CircularProgressIndicator(Modifier.height(20.dp).width(20.dp), strokeWidth = 2.dp) }
                list.isEmpty() -> Text("没有在线会话")
                else -> Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text("共 ${list.size} 个在线会话，点击可踢出", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    LazyColumn(Modifier.height(280.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        items(list, key = { it.id }) { s ->
                            Card(
                                shape = RoundedCornerShape(10.dp),
                                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceVariant),
                                modifier = Modifier.fillMaxWidth().clickable(enabled = !s.current) {
                                    scope.launch {
                                        apiGuard(onError = {}) { Api.revokeSession(clientFor(server), s.id) }
                                        reload++
                                    }
                                },
                            ) {
                                Column(Modifier.padding(10.dp)) {
                                    Row(verticalAlignment = Alignment.CenterVertically) {
                                        Text(
                                            if (s.client == "app") "APP · ${s.device_name.ifBlank { "Android" }}" else "Web",
                                            fontWeight = FontWeight.SemiBold, fontSize = 13.sp,
                                            color = if (s.client == "app") MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurface,
                                        )
                                        Spacer(Modifier.width(8.dp))
                                        if (s.current) Text("（本机）", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                    }
                                    Text("${s.ip} · ${s.expires}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                    Text(s.ua.take(50), fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
                                }
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = {
                scope.launch {
                    apiGuard(onError = {}) { Api.revokeOthers(clientFor(server)) }
                    reload++
                }
            }) { Text("退出其他设备") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("关闭") } },
    )
}

@Composable
fun AlertsScreen(server: ServerInfo, onClose: () -> Unit) {
    val scope = rememberCoroutineScope()
    val client = clientFor(server)
    var tab by remember { mutableIntStateOf(0) }
    var rules by remember(server.host) { mutableStateOf<List<cn.hanyuxin.panel.data.AlertRule>?>(null) }
    var events by remember(server.host) { mutableStateOf<cn.hanyuxin.panel.data.AlertEventsPage?>(null) }
    var err by remember { mutableStateOf<String?>(null) }
    var reload by remember { mutableIntStateOf(0) }
    var createOpen by remember { mutableStateOf(false) }
    var editTarget by remember { mutableStateOf<cn.hanyuxin.panel.data.AlertRule?>(null) }
    var delTarget by remember { mutableStateOf<cn.hanyuxin.panel.data.AlertRule?>(null) }
    var busyId by remember { mutableStateOf<Long?>(null) }

    LaunchedEffect(server.host, reload) {
        apiGuard(onError = { err = it.message }) {
            rules = Api.alertRules(client)
            events = Api.alertEvents(client)
        }
    }

    Column(Modifier.fillMaxSize()) {
        Row(Modifier.padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
            Text("消息中心", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            IconButton(onClick = { reload++ }) { Icon(Icons.Outlined.Refresh, "刷新") }
            TextButton(onClick = onClose) { Text("关闭") }
        }
        ErrorBanner(err)
        TabRow(selectedTabIndex = tab) {
            Tab(tab == 0, onClick = { tab = 0 }, text = { Text("告警规则") })
            Tab(tab == 1, onClick = { tab = 1 }, text = { Text("触发历史") })
        }
        if (tab == 0) {
            Row(
                Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    "阈值规则每 30 秒评估一次",
                    fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f),
                )
                IconButton(onClick = { createOpen = true }) { Icon(Icons.Outlined.Add, "新建规则", tint = MaterialTheme.colorScheme.primary) }
            }
            val rl = rules
            when {
                rl == null -> LoadingView()
                rl.isEmpty() -> EmptyView("还没有告警规则，点右上角 + 新建")
                else -> LazyColumn(
                    Modifier.fillMaxSize(),
                    contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 16.dp, vertical = 4.dp),
                    verticalArrangement = Arrangement.spacedBy(10.dp),
                ) {
                    items(rl, key = { it.id }) { r ->
                        PanelCard {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Column(Modifier.weight(1f)) {
                                    Text(r.name, fontWeight = FontWeight.SemiBold)
                                    Text(
                                        (METRIC_LABEL[r.metric] ?: r.metric) + " " + r.op + " " + r.threshold + " · 持续 " + r.duration_min + " 分 · 冷却 " + r.cooldown_min + " 分",
                                        fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                                    )
                                }
                                IconButton(onClick = { editTarget = r }) { Icon(Icons.Outlined.Edit, "编辑", tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(18.dp)) }
                                IconButton(onClick = { delTarget = r }) { Icon(Icons.Outlined.Delete, "删除", tint = MaterialTheme.colorScheme.error, modifier = Modifier.size(18.dp)) }
                                Switch(
                                    r.enabled,
                                    onCheckedChange = { _ ->
                                        busyId = r.id
                                        scope.launch {
                                            apiGuard(onError = { err = it.message }) { Api.alertRuleToggle(client, r.id) }
                                            busyId = null
                                            reload++
                                        }
                                    },
                                    enabled = busyId != r.id,
                                )
                            }
                        }
                    }
                }
            }
        } else {
            val p2 = events
            when {
                p2 == null -> LoadingView()
                p2.items.isEmpty() -> EmptyView("暂无告警事件")
                else -> LazyColumn(
                    verticalArrangement = Arrangement.spacedBy(10.dp),
                    contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 16.dp),
                ) {
                    item { Text("共 " + p2.total + " 条", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                    items(p2.items, key = { it.id }) { e ->
                        PanelCard {
                            Text(e.rule_name, fontWeight = FontWeight.SemiBold)
                            Text(
                                listOfNotNull(e.metric.takeIf { it.isNotBlank() }, e.value.takeIf { it.isNotBlank() }).joinToString(" = "),
                                fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                            if (e.message.isNotBlank()) Text(e.message, fontSize = 12.sp)
                            Text(e.created_at.take(19).replace('T', ' '), fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                }
            }
        }
    }

    if (createOpen || editTarget != null) {
        val editing = editTarget
        var name by remember { mutableStateOf(editing?.name ?: "") }
        var metric by remember { mutableStateOf(editing?.metric ?: "cpu") }
        var op by remember { mutableStateOf(editing?.op ?: ">") }
        var threshold by remember { mutableStateOf((editing?.threshold ?: 90.0).toString()) }
        var duration by remember { mutableStateOf((editing?.duration_min ?: 3).toString()) }
        var cooldown by remember { mutableStateOf((editing?.cooldown_min ?: 30).toString()) }
        AlertDialog(
            onDismissRequest = { createOpen = false; editTarget = null },
            title = { Text(if (editing != null) "编辑规则" else "新建告警规则") },
            text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedTextField(name, { name = it }, label = { Text("规则名称") }, singleLine = true)
                    Text("监控指标", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                        FilterChip(metric == "cpu", { metric = "cpu" }, label = { Text("CPU") })
                        FilterChip(metric == "mem", { metric = "mem" }, label = { Text("内存") })
                        FilterChip(metric == "disk", { metric = "disk" }, label = { Text("磁盘") })
                        FilterChip(metric == "load1", { metric = "load1" }, label = { Text("负载") })
                    }
                    Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                        FilterChip(op == ">", { op = ">" }, label = { Text("大于") })
                        FilterChip(op == "<", { op = "<" }, label = { Text("小于") })
                        OutlinedTextField(threshold, { threshold = it }, label = { Text("阈值") }, singleLine = true, modifier = Modifier.weight(1f))
                    }
                    OutlinedTextField(duration, { duration = it }, label = { Text("持续(分钟)，0=立即") }, singleLine = true)
                    OutlinedTextField(cooldown, { cooldown = it }, label = { Text("冷却(分钟)") }, singleLine = true)
                }
            },
            confirmButton = {
                TextButton(onClick = {
                    val f = cn.hanyuxin.panel.data.AlertRule(
                        id = editing?.id ?: 0, name = name.trim(), metric = metric, op = op,
                        threshold = threshold.toDoubleOrNull() ?: 90.0,
                        duration_min = duration.toIntOrNull() ?: 3,
                        cooldown_min = cooldown.toIntOrNull() ?: 30,
                        node_id = editing?.node_id ?: 0,
                    )
                    if (f.name.isBlank()) { err = "请填写规则名称"; return@TextButton }
                    scope.launch {
                        var ok = false
                        apiGuard(onError = { err = it.message }) {
                            if (editing != null) Api.alertRuleUpdate(client, editing.id, f)
                            else Api.alertRuleCreateFull(client, f)
                            ok = true
                        }
                        if (ok) {
                            err = null
                            createOpen = false
                            editTarget = null
                            reload++
                        }
                    }
                }) { Text("保存") }
            },
            dismissButton = { TextButton(onClick = { createOpen = false; editTarget = null }) { Text("取消") } },
        )
    }

    delTarget?.let { r2 ->
        AlertDialog(
            onDismissRequest = { delTarget = null },
            title = { Text("删除规则 " + r2.name) },
            text = { Text("确认删除该告警规则？执行历史将保留。") },
            confirmButton = {
                TextButton(onClick = {
                    scope.launch {
                        var ok = false
                        apiGuard(onError = { err = it.message }) { Api.alertRuleDelete(client, r2.id); ok = true }
                        if (ok) { delTarget = null; reload++ }
                    }
                }) { Text("删除", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { delTarget = null }) { Text("取消") } },
        )
    }
}

private val METRIC_LABEL = mapOf("cpu" to "CPU 使用率(%)", "mem" to "内存使用率(%)", "disk" to "磁盘使用率(%)", "load1" to "1分钟负载")


/** 流式计算 SHA256（更新包完整性校验）。 */
fun sha256Of(input: java.io.InputStream): String {
	val md = MessageDigest.getInstance("SHA-256")
	val buf = ByteArray(8192)
	var n: Int
	while (input.read(buf).also { n = it } > 0) md.update(buf, 0, n)
	return md.digest().joinToString("") { "%02x".format(it) }
}