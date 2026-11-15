// AppStoreScreen.kt —— 软件商店：模板安装（含参数表单）+ 已装应用管理（升级/卸载/强卸两段式）。
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
import androidx.compose.material.icons.outlined.Build
import androidx.compose.material.icons.outlined.Code
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Extension
import androidx.compose.material.icons.outlined.Language
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material.icons.outlined.Storage
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
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
    var reload by remember { mutableIntStateOf(0) }

    Column(Modifier.fillMaxSize()) {
        Column(Modifier.padding(horizontal = 16.dp)) {
            Spacer(Modifier.height(4.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("软件商店", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)
                    Text("应用模板一键安装 · 已装应用统一管理", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                IconButton(onClick = { reload++ }) { Icon(Icons.Outlined.Refresh, "刷新") }
            }
            Spacer(Modifier.height(10.dp))
            Segmented(listOf("应用模板", "已安装"), tab, onSelect = { tab = it })
            Spacer(Modifier.height(12.dp))
        }
        if (tab == 0) TemplatesTab(server, reload) else InstalledTab(server, reload, onReload = { reload++ })
    }
}

/** 模板类别 → 图标（key/kind 启发式）。 */
private fun kindIcon(kind: String): ImageVector = when {
    kind.contains("web") || kind.contains("site") -> Icons.Outlined.Language
    kind.contains("runtime") || kind.contains("lang") || kind.contains("php") -> Icons.Outlined.Code
    kind.contains("db") || kind.contains("data") || kind.contains("sql") -> Icons.Outlined.Storage
    kind.contains("tool") || kind.contains("sys") -> Icons.Outlined.Build
    else -> Icons.Outlined.Extension
}

/** 模板类别 → 语义色。 */
private fun kindTint(kind: String): Color = when {
    kind.contains("web") || kind.contains("site") -> SkyBlue
    kind.contains("runtime") || kind.contains("lang") || kind.contains("php") -> Emerald
    kind.contains("db") || kind.contains("data") || kind.contains("sql") -> Amber
    kind.contains("tool") || kind.contains("sys") -> Violet
    else -> Color(0xFF2DD4BF)
}

@Composable
private fun TemplatesTab(server: ServerInfo, reload: Int) {
    var templates by remember(server.host) { mutableStateOf<List<AppTemplate>?>(null) }
    var err by remember { mutableStateOf<String?>(null) }
    var installing by remember { mutableStateOf<AppTemplate?>(null) }
    var query by remember { mutableStateOf("") }

    LaunchedEffect(server.host, reload) {
        apiGuard(onError = { err = it.message }) { Api.templates(clientFor(server)) }?.let { templates = it; err = null }
    }

    Column(Modifier.fillMaxSize().padding(horizontal = 16.dp)) {
        SearchField(query, { query = it }, "搜索模板名称 / 类别")
        Spacer(Modifier.height(8.dp))
        ErrorBanner(err)
        val list = templates
        when {
            list == null -> LoadingView()
            list.isEmpty() -> EmptyView("暂无模板")
            else -> {
                val q = query.trim().lowercase()
                val filtered = list.filter {
                    q.isBlank() || it.name.lowercase().contains(q) || it.key.lowercase().contains(q) ||
                        it.desc.lowercase().contains(q) || it.kind.lowercase().contains(q)
                }
                LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    if (filtered.isEmpty()) {
                        item { EmptyView("没有匹配「${query.trim()}」的模板") }
                    } else {
                        item {
                            Text(
                                "共 ${filtered.size} 个模板",
                                fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                        items(filtered, key = { it.key }) { t ->
                            Card(
                                shape = RoundedCornerShape(16.dp),
                                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                                modifier = Modifier.fillMaxWidth().clickable { installing = t },
                            ) {
                                Column(Modifier.padding(14.dp)) {
                                    Row(verticalAlignment = Alignment.CenterVertically) {
                                        IconBadge(kindIcon(t.kind), kindTint(t.kind), badge = 40.dp, iconSize = 21.dp)
                                        Spacer(Modifier.width(11.dp))
                                        Column(Modifier.weight(1f)) {
                                            Text(t.name, fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                                            Text(
                                                t.desc,
                                                fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                                                maxLines = 2, overflow = TextOverflow.Ellipsis,
                                            )
                                        }
                                    }
                                    Spacer(Modifier.height(10.dp))
                                    Row(
                                        Modifier.fillMaxWidth(),
                                        horizontalArrangement = Arrangement.SpaceBetween,
                                        verticalAlignment = Alignment.CenterVertically,
                                    ) {
                                        Row(
                                            horizontalArrangement = Arrangement.spacedBy(6.dp),
                                            verticalAlignment = Alignment.CenterVertically,
                                            modifier = Modifier.weight(1f, fill = false),
                                        ) {
                                            TagChip(t.kind.ifBlank { "app" }, kindTint(t.kind))
                                            TagChip(t.default_version.ifBlank { "默认版本" }, SkyBlue)
                                        }
                                        Button(
                                            onClick = { installing = t },
                                            shape = RoundedCornerShape(10.dp),
                                            contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 16.dp, vertical = 5.dp),
                                        ) {
                                            Text("安装", fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
                                        }
                                    }
                                }
                            }
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

/** 应用显示名：name/app_key 均空的存量记录兜底。 */
private fun dispName(a: AppInst): String = a.name.ifBlank { a.app_key.ifBlank { "应用 #${a.id}" } }

@Composable
private fun InstalledTab(server: ServerInfo, reload: Int, onReload: () -> Unit) {
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()
    var apps by remember(server.host) { mutableStateOf<List<AppInst>?>(null) }
    var err by remember { mutableStateOf<String?>(null) }
    var busyId by remember { mutableStateOf<Long?>(null) }
    var forceTarget by remember { mutableStateOf<AppInst?>(null) }
    var strongConfirm by remember { mutableStateOf<AppInst?>(null) }

    fun toast(s: String) = android.widget.Toast.makeText(ctx, s, android.widget.Toast.LENGTH_SHORT).show()

    LaunchedEffect(server.host, reload) {
        apiGuard(onError = { err = it.message }) { Api.installedApps(clientFor(server)) }?.let { apps = it; err = null }
    }

    Column(Modifier.fillMaxSize().padding(horizontal = 16.dp)) {
        ErrorBanner(err)
        val list = apps
        when {
            list == null -> LoadingView()
            list.isEmpty() -> EmptyView("还没有安装应用，去「应用模板」页一键安装")
            else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                item {
                    Text(
                        "已安装 ${list.size} 个应用",
                        fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                items(list, key = { it.id }) { a ->
                    Card(
                        shape = RoundedCornerShape(16.dp),
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                        modifier = Modifier.fillMaxWidth(),
                    ) {
                        Column(Modifier.padding(14.dp)) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                IconBadge(kindIcon(a.kind), kindTint(a.kind), badge = 40.dp, iconSize = 21.dp)
                                Spacer(Modifier.width(11.dp))
                                Column(Modifier.weight(1f)) {
                                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                                        Text(dispName(a), fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                                        if (a.tool) TagChip("工具", Slate)
                                    }
                                    val meta = listOfNotNull(a.app_key, a.kind, a.version.takeIf { it.isNotBlank() }).joinToString(" · ")
                                    if (meta.isNotBlank()) {
                                        Text(
                                            meta,
                                            fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                                            maxLines = 1, overflow = TextOverflow.Ellipsis,
                                        )
                                    }
                                }
                            }
                            Spacer(Modifier.height(10.dp))
                            Row(
                                Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.spacedBy(8.dp, Alignment.End),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                if (busyId == a.id) {
                                    CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                                } else {
                                    FilledTonalButton(
                                        onClick = {
                                            busyId = a.id
                                            scope.launch {
                                                runCatching { Api.upgradeApp(clientFor(server), a.id) }
                                                    .onSuccess { toast("升级任务已启动") }
                                                    .onFailure { toast(it.message ?: "升级失败") }
                                                busyId = null; onReload()
                                            }
                                        },
                                        shape = RoundedCornerShape(10.dp),
                                        contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 14.dp, vertical = 4.dp),
                                        colors = ButtonDefaults.filledTonalButtonColors(
                                            containerColor = MaterialTheme.colorScheme.primaryContainer,
                                            contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
                                        ),
                                    ) { Text("升级", fontSize = 12.sp, fontWeight = FontWeight.SemiBold) }
                                    // 卸载：错误色圆形图标按钮
                                    Box(
                                        Modifier
                                            .size(32.dp)
                                            .background(MaterialTheme.colorScheme.error.copy(alpha = 0.14f), CircleShape)
                                            .clickable { forceTarget = a },
                                        contentAlignment = Alignment.Center,
                                    ) {
                                        Icon(
                                            Icons.Outlined.Delete, "卸载",
                                            tint = MaterialTheme.colorScheme.error, modifier = Modifier.size(16.dp),
                                        )
                                    }
                                }
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
            title = { Text("卸载 ${dispName(a)}") },
            text = { Text("将卸载应用 ${dispName(a)}（${a.app_key.ifBlank { "未知类型" }}）。涉及面板依赖的应用会要求二次强制确认。") },
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
                            busyId = null; onReload()
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
            title = { Text("强制卸载 ${dispName(a)}？") },
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
                        } finally { busyId = null; onReload() }
                    }
                }) { Text("强制卸载", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { strongConfirm = null }) { Text("取消") } },
        )
    }
}
