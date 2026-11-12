// MeScreens.kt —— 「我的」（服务器信息/设备管理/生物识别锁/检查更新/退出）与消息中心。
package cn.hanyuxin.panel.ui

import android.content.Intent
import android.net.Uri
import androidx.biometric.BiometricManager
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Devices
import androidx.compose.material.icons.outlined.Fingerprint
import androidx.compose.material.icons.outlined.Notifications
import androidx.compose.material.icons.outlined.QrCodeScanner
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material.icons.outlined.SwapHoriz
import androidx.compose.material.icons.outlined.SystemUpdate
import androidx.compose.material.icons.outlined.Logout
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
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
import kotlinx.coroutines.launch

@Composable
fun MeScreen(server: ServerInfo) {
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()
    var info by remember(server.host) { mutableStateOf<PanelInfo?>(null) }
    var bio by remember { mutableStateOf(ServerStore.bioEnabled()) }
    var showDevices by remember { mutableStateOf(false) }
    var update by remember { mutableStateOf<UpdateInfo?>(null) }
    var checking by remember { mutableStateOf(false) }

    LaunchedEffect(server.host) {
        apiGuard { Api.panelInfo(clientFor(server)) }?.let { info = it }
    }

    fun toast(s: String) = android.widget.Toast.makeText(ctx, s, android.widget.Toast.LENGTH_SHORT).show()

    Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Text("我的", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)

        PanelCard {
            Text(server.name.ifBlank { server.host }, fontWeight = FontWeight.SemiBold)
            Text(server.host, fontSize = 12.sp, fontFamily = FontFamily.Monospace, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.height(6.dp))
            info?.let { KeyValue("面板版本", it.version, mono = true) }
            info?.let { KeyValue("服务端", "${it.os}/${it.arch} · ${if (it.tls) "HTTPS" else "HTTP"}") }
        }

        MenuRow(Icons.Outlined.Notifications, "消息中心", "告警规则触发事件") { Router.push(Router.Route.Alerts(server) { Router.pop() }) }
        MenuRow(Icons.Outlined.Devices, "登录设备", "查看与踢出在线会话") { showDevices = true }
        MenuRow(Icons.Outlined.SystemUpdate, "检查更新", "APP 自更新") {
            checking = true
            scope.launch {
                apiGuard(onError = { toast(it.message ?: "检查失败") }) { Api.checkUpdate(server) }?.let { update = it }
                checking = false
            }
        }
        Row(
            Modifier.fillMaxWidth().clickable {
                val bm = BiometricManager.from(ctx)
                val can = bm.canAuthenticate(BiometricManager.Authenticators.BIOMETRIC_WEAK or BiometricManager.Authenticators.DEVICE_CREDENTIAL)
                if (can != BiometricManager.BIOMETRIC_SUCCESS) {
                    toast("设备未设置锁屏密码或指纹，无法开启")
                } else {
                    bio = !bio
                    ServerStore.setBioEnabled(bio)
                    toast(if (bio) "生物识别锁已开启" else "生物识别锁已关闭")
                }
            }.padding(vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(Icons.Outlined.Fingerprint, null, tint = MaterialTheme.colorScheme.primary)
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Text("生物识别锁", fontWeight = FontWeight.Medium)
                Text("每次打开 APP 需指纹或锁屏凭据解锁", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Switch(checked = bio, onCheckedChange = null)
        }
        MenuRow(Icons.Outlined.QrCodeScanner, "扫码添加服务器", "扫 Web 登录页二维码") {
            Router.push(Router.Route.Scanner(onQr = { handleScan(it) }, onClose = { Router.pop() }))
        }
        MenuRow(Icons.Outlined.SwapHoriz, "切换服务器", "返回服务器列表") { Session.leave() }
        MenuRow(Icons.Outlined.Logout, "退出登录", "清除本机会话令牌", danger = true) {
            scope.launch {
                apiGuard { Api.logout(clientFor(server), server.host) }
                Session.leave()
                toast("已退出登录")
            }
        }
        Spacer(Modifier.height(8.dp))
        Text("寒语馨面板 APP · Kotlin/Compose · 设计文档 docs/APP.md", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
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
                if (u.has_update && u.apk_url.isNotBlank()) {
                    TextButton(onClick = {
                        ctx.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(u.apk_url)))
                        update = null
                    }) { Text("去下载") }
                } else {
                    TextButton(onClick = { update = null }) { Text("知道了") }
                }
            },
            dismissButton = if (u.has_update) ({ TextButton(onClick = { update = null }) { Text("以后再说") } }) else null,
        )
    }
}

@Composable
private fun MenuRow(icon: androidx.compose.ui.graphics.vector.ImageVector, title: String, sub: String, danger: Boolean = false, onClick: () -> Unit) {
    Row(
        Modifier.fillMaxWidth().clickable { onClick() }.padding(vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, null, tint = if (danger) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary)
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f)) {
            Text(title, fontWeight = FontWeight.Medium, color = if (danger) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface)
            Text(sub, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
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
    var events by remember(server.host) { mutableStateOf<cn.hanyuxin.panel.data.AlertEventsPage?>(null) }
    var err by remember { mutableStateOf<String?>(null) }
    var reload by remember { mutableStateOf(0) }

    LaunchedEffect(server.host, reload) {
        apiGuard(onError = { err = it.message }) { Api.alertEvents(clientFor(server)) }?.let { events = it; err = null }
    }

    Column(Modifier.fillMaxSize().padding(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("消息中心", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            IconButton(onClick = { reload++ }) { Icon(Icons.Outlined.Refresh, "刷新") }
            TextButton(onClick = onClose) { Text("关闭") }
        }
        ErrorBanner(err)
        val p = events
        when {
            p == null -> LoadingView()
            p.items.isEmpty() -> EmptyView("暂无告警事件")
            else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                item { Text("共 ${p.total} 条", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant) }
                items(p.items, key = { it.id }) { e ->
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
