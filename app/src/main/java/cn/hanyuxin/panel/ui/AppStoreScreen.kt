// AppStoreScreen.kt —— 软件商店：模板安装（含参数表单）+ 已装应用管理（升级/卸载/强卸两段式）。
package cn.hanyuxin.panel.ui

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
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.Api
import cn.hanyuxin.panel.data.ApiException
import cn.hanyuxin.panel.data.AppInst
import cn.hanyuxin.panel.data.AppTemplate
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.clientFor
import kotlinx.coroutines.launch

@Composable
fun AppStoreScreen(server: ServerInfo) {
    var tab by remember { mutableIntStateOf(0) }
    Column(Modifier.fillMaxSize()) {
        TabRow(selectedTabIndex = tab) {
            Tab(tab == 0, onClick = { tab = 0 }, text = { Text("应用模板") })
            Tab(tab == 1, onClick = { tab = 1 }, text = { Text("已安装") })
        }
        when (tab) {
            0 -> TemplatesTab(server)
            else -> InstalledTab(server)
        }
    }
}

@Composable
private fun TemplatesTab(server: ServerInfo) {
    val scope = rememberCoroutineScope()
    var templates by remember(server.host) { mutableStateOf<List<AppTemplate>?>(null) }
    var err by remember { mutableStateOf<String?>(null) }
    var installing by remember { mutableStateOf<AppTemplate?>(null) }

    LaunchedEffect(server.host) {
        apiGuard(onError = { err = it.message }) { Api.templates(clientFor(server)) }?.let { templates = it; err = null }
    }

    Column(Modifier.fillMaxSize().padding(16.dp)) {
        ErrorBanner(err)
        val list = templates
        when {
            list == null -> LoadingView()
            list.isEmpty() -> EmptyView("暂无模板")
            else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                items(list, key = { it.key }) { t ->
                    Card(
                        shape = RoundedCornerShape(14.dp),
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                        modifier = Modifier.fillMaxWidth().clickable { installing = t },
                    ) {
                        Row(Modifier.padding(14.dp), verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text(t.name, fontWeight = FontWeight.SemiBold)
                                Text(t.desc, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                Text(
                                    "${t.kind} · ${t.default_version.ifBlank { "默认版本" }}",
                                    fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                            Text("安装", color = MaterialTheme.colorScheme.primary, fontWeight = FontWeight.SemiBold)
                        }
                    }
                }
            }
        }
    }

    installing?.let { t -> InstallDialog(server, t, onDismiss = { installing = null }) }
}

@Composable
private fun InstallDialog(server: ServerInfo, tpl: AppTemplate, onDismiss: () -> Unit) {
    val scope = rememberCoroutineScope()
    var name by remember { mutableStateOf(tpl.key) }
    var params by remember(tpl.key) { mutableStateOf(tpl.params.associate { it.key to it.def }) }
    var busy by remember { mutableStateOf(false) }
    var err by remember { mutableStateOf<String?>(null) }

    AlertDialog(
        onDismissRequest = { if (!busy) onDismiss() },
        title = { Text("安装 ${tpl.name}") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(tpl.desc, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                OutlinedTextField(name, { name = it }, label = { Text("实例名（小写字母数字短横线）") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
                tpl.params.forEach { p ->
                    OutlinedTextField(
                        params[p.key] ?: "", { params = params + (p.key to it) },
                        label = { Text(p.label + if (p.required) " * " else "") },
                        modifier = Modifier.fillMaxWidth(),
                        singleLine = true,
                    )
                }
                ErrorBanner(err)
                Text("安装为长任务，可在「已安装」页查看状态", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        },
        confirmButton = {
            if (busy) CircularProgressIndicator(Modifier.height(20.dp).width(20.dp), strokeWidth = 2.dp)
            else Button(onClick = {
                val missing = tpl.params.firstOrNull { it.required && (params[it.key] ?: "").isBlank() }
                if (missing != null) { err = "请填写 ${missing.label}"; return@Button }
                busy = true; err = null
                scope.launch {
                    try {
                        Api.installApp(clientFor(server), tpl.key, name.trim().lowercase(), tpl.default_version, params)
                        onDismiss()
                    } catch (e: ApiException) { err = e.message } catch (e: Exception) { err = e.message ?: "安装请求失败" }
                    finally { busy = false }
                }
            }) { Text("开始安装") }
        },
        dismissButton = { TextButton(onClick = { if (!busy) onDismiss() }) { Text("取消") } },
    )
}

@Composable
private fun InstalledTab(server: ServerInfo) {
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()
    var apps by remember(server.host) { mutableStateOf<List<AppInst>?>(null) }
    var err by remember { mutableStateOf<String?>(null) }
    var busyId by remember { mutableStateOf<Long?>(null) }
    var forceTarget by remember { mutableStateOf<AppInst?>(null) }
    var strongConfirm by remember { mutableStateOf<AppInst?>(null) }
    var reload by remember { mutableIntStateOf(0) }

    fun toast(s: String) = android.widget.Toast.makeText(ctx, s, android.widget.Toast.LENGTH_SHORT).show()

    LaunchedEffect(server.host, reload) {
        apiGuard(onError = { err = it.message }) { Api.installedApps(clientFor(server)) }?.let { apps = it; err = null }
    }

    Column(Modifier.fillMaxSize().padding(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("已安装应用", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            IconButton(onClick = { reload++ }) { Icon(Icons.Outlined.Refresh, "刷新") }
        }
        ErrorBanner(err)
        val list = apps
        when {
            list == null -> LoadingView()
            list.isEmpty() -> EmptyView("还没有安装应用")
            else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                items(list, key = { it.id }) { a ->
                    Card(
                        shape = RoundedCornerShape(14.dp),
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Row(Modifier.padding(14.dp), verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text(a.name, fontWeight = FontWeight.SemiBold)
                                Text(
                                    listOfNotNull(a.app_key, a.kind, a.version.takeIf { it.isNotBlank() }).joinToString(" · "),
                                    fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                            if (busyId == a.id) {
                                CircularProgressIndicator(Modifier.height(20.dp).width(20.dp), strokeWidth = 2.dp)
                            } else {
                                TextButton(onClick = {
                                    busyId = a.id
                                    scope.launch {
                                        runCatching { Api.upgradeApp(clientFor(server), a.id) }
                                            .onSuccess { toast("升级任务已启动") }
                                            .onFailure { toast(it.message ?: "升级失败") }
                                        busyId = null; reload++
                                    }
                                }) { Text("升级") }
                                IconButton(onClick = { forceTarget = a }) { Icon(Icons.Outlined.Delete, "卸载", tint = MaterialTheme.colorScheme.error) }
                            }
                        }
                    }
                }
            }
        }
    }

    forceTarget?.let { a ->
        AlertDialog(
            onDismissRequest = { forceTarget = null },
            title = { Text("卸载 ${a.name}") },
            text = { Text("将卸载应用 ${a.name}（${a.app_key}）。涉及面板依赖的应用会要求二次强制确认。") },
            confirmButton = {
                TextButton(onClick = {
                    busyId = a.id; forceTarget = null
                    scope.launch {
                        try {
                            Api.uninstallApp(clientFor(server), a.id, force = false, removeData = false)
                            toast("卸载任务已启动")
                        } catch (e: ApiException) {
                            // 面板依赖保护（v0.40.7）：拒绝普通卸载 → 二段式强卸确认
                            err = e.message
                            strongConfirm = a
                        } catch (e: Exception) {
                            toast(e.message ?: "卸载失败")
                        } finally {
                            busyId = null; reload++
                        }
                    }
                }) { Text("卸载") }
            },
            dismissButton = { TextButton(onClick = { forceTarget = null }) { Text("取消") } },
        )
    }

    strongConfirm?.let { a ->
        AlertDialog(
            onDismissRequest = { strongConfirm = null },
            title = { Text("强制卸载 ${a.name}？") },
            text = { Text(err ?: "面板正在使用该应用，强卸可能导致面板域名与网站无法访问。确认不受影响才继续。") },
            confirmButton = {
                TextButton(onClick = {
                    busyId = a.id; strongConfirm = null
                    scope.launch {
                        try {
                            Api.uninstallApp(clientFor(server), a.id, force = true, removeData = false)
                            toast("强制卸载任务已启动")
                        } catch (e: Exception) {
                            toast(e.message ?: "强卸失败")
                        } finally { busyId = null; reload++ }
                    }
                }) { Text("强制卸载", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { strongConfirm = null }) { Text("取消") } },
        )
    }
}
