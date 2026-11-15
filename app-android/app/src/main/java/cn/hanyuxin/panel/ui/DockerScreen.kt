// DockerScreen.kt —— Docker 管理：状态概览 + 容器（启停/重启/删除/日志）+ 镜像（列表/拉取/清理）。
package cn.hanyuxin.panel.ui

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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CleaningServices
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Download
import androidx.compose.material.icons.outlined.PlayCircle
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material.icons.outlined.RestartAlt
import androidx.compose.material.icons.outlined.StopCircle
import androidx.compose.material.icons.outlined.ViewInAr
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.Api
import cn.hanyuxin.panel.data.DockerOverview
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.clientFor
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonPrimitive

/** docker 原生 JSON 字段取字符串（大写开头字段：ID/Names/Image/State/Status/Ports 等）。 */
internal fun jstr(m: Map<String, JsonElement>, key: String): String =
    (m[key] as? JsonPrimitive)?.content ?: ""

@Composable
fun DockerScreen(server: ServerInfo) {
    val scope = rememberCoroutineScope()
    var overview by remember { mutableStateOf<DockerOverview?>(null) }
    var tab by remember { mutableStateOf("容器") }
    var reload by remember { mutableStateOf(0) }
    var err by remember { mutableStateOf<String?>(null) }
    var containers by remember { mutableStateOf<List<Map<String, JsonElement>>?>(null) }
    var images by remember { mutableStateOf<List<Map<String, JsonElement>>?>(null) }
    var busy by remember { mutableStateOf(false) }
    var query by remember { mutableStateOf("") }
    var logs by remember { mutableStateOf<Pair<String, String>?>(null) }
    var pullOpen by remember { mutableStateOf(false) }
    var pullImage by remember { mutableStateOf("") }
    var pullOut by remember { mutableStateOf<String?>(null) }

    fun act(block: suspend () -> Unit) {
        if (busy) return
        busy = true
        scope.launch {
            try { block(); reload++ } catch (e: Exception) { err = e.message ?: "操作失败" } finally { busy = false }
        }
    }

    fun viewLogs(name: String) {
        scope.launch {
            apiGuard(onError = { err = it.message }) { Api.dockerLogs(clientFor(server), name) }?.let { logs = name to it }
        }
    }

    LaunchedEffect(server.host, reload, tab) {
        apiGuard(onError = { err = it.message }) { Api.dockerOverview(clientFor(server)) }?.let { overview = it }
        if (tab == "容器") {
            apiGuard(onError = { err = it.message }) { Api.dockerContainers(clientFor(server)) }?.let { containers = it }
        } else {
            apiGuard(onError = { err = it.message }) { Api.dockerImages(clientFor(server)) }?.let { images = it }
        }
    }

    Column(Modifier.fillMaxSize()) {
        Column(Modifier.padding(horizontal = 16.dp)) {
            Spacer(Modifier.height(4.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("Docker 管理", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)
                    overview?.let { o ->
                        val ver = jstr(o.version, "Version")
                        Text(
                            when {
                                !o.installed -> "引擎未安装"
                                ver.isBlank() -> "运行 ${o.running} · 已停止 ${o.stopped} · 镜像 ${o.images}"
                                else -> "引擎 $ver · 运行 ${o.running} · 已停止 ${o.stopped} · 镜像 ${o.images}"
                            },
                            fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
                IconButton(onClick = { reload++ }) { Icon(Icons.Outlined.Refresh, "刷新") }
            }
            if (busy) {
                LinearProgressIndicator(
                    Modifier.fillMaxWidth().padding(top = 2.dp).height(2.dp),
                    color = MaterialTheme.colorScheme.primary,
                    trackColor = MaterialTheme.colorScheme.surfaceVariant,
                )
            }
            Spacer(Modifier.height(10.dp))
            ErrorBanner(err)
            Spacer(Modifier.height(4.dp))
        }

        val ov = overview
        if (ov != null && !ov.installed) {
            // 未安装：引导卡
            Column(Modifier.padding(horizontal = 16.dp)) {
                Card(
                    shape = RoundedCornerShape(18.dp),
                    colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                ) {
                    Column(
                        Modifier.fillMaxWidth().padding(horizontal = 24.dp, vertical = 30.dp),
                        horizontalAlignment = Alignment.CenterHorizontally,
                    ) {
                        Box(
                            Modifier.size(64.dp).background(MaterialTheme.colorScheme.primary.copy(alpha = 0.12f), RoundedCornerShape(20.dp)),
                            contentAlignment = Alignment.Center,
                        ) {
                            Icon(Icons.Outlined.ViewInAr, null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(32.dp))
                        }
                        Spacer(Modifier.height(14.dp))
                        Text("Docker 未安装", fontWeight = FontWeight.SemiBold, fontSize = 16.sp)
                        Spacer(Modifier.height(4.dp))
                        Text(
                            "前往软件商店安装 Docker 引擎后，\n即可在此管理容器与镜像",
                            fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                            lineHeight = 18.sp,
                        )
                        Spacer(Modifier.height(16.dp))
                        Button(
                            onClick = { Router.push(Router.Route.AppStore(server)) },
                            shape = RoundedCornerShape(11.dp),
                        ) {
                            Icon(Icons.Outlined.Download, null, modifier = Modifier.size(16.dp))
                            Spacer(Modifier.width(6.dp))
                            Text("去软件商店安装", fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
                        }
                    }
                }
            }
        } else {
            // 概览统计 + 分段器 + 搜索
            Column(Modifier.padding(horizontal = 16.dp)) {
                ov?.let { o ->
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        StatTile("运行中", o.running.toString(), OkGreen, Modifier.weight(1f))
                        StatTile("已停止", o.stopped.toString(), IdleGray, Modifier.weight(1f))
                        StatTile("镜像", o.images.toString(), SkyBlue, Modifier.weight(1f))
                    }
                    Spacer(Modifier.height(10.dp))
                }
                Segmented(listOf("容器", "镜像"), if (tab == "容器") 0 else 1, onSelect = { tab = if (it == 0) "容器" else "镜像" })
                Spacer(Modifier.height(10.dp))
                SearchField(query, { query = it }, if (tab == "容器") "搜索容器名 / 镜像" else "搜索镜像仓库")
                Spacer(Modifier.height(10.dp))
            }

            if (tab == "容器") {
                val q = query.trim().lowercase()
                val list = containers?.filter { c ->
                    val n = jstr(c, "Names").removePrefix("/").lowercase()
                    val img = jstr(c, "Image").lowercase()
                    q.isBlank() || n.contains(q) || img.contains(q)
                }
                LazyColumn(
                    Modifier.fillMaxSize().padding(horizontal = 16.dp),
                    verticalArrangement = Arrangement.spacedBy(10.dp),
                ) {
                    when {
                        list == null -> item { LoadingView() }
                        list.isEmpty() -> item { EmptyView(if (query.isBlank()) "没有容器" else "没有匹配的容器") }
                        else -> items(list) { c -> ContainerCard(c, server, busy, onViewLogs = { viewLogs(it) }, onAction = { act(it) }) }
                    }
                }
            } else {
                LazyColumn(
                    Modifier.fillMaxSize().padding(horizontal = 16.dp),
                    verticalArrangement = Arrangement.spacedBy(10.dp),
                ) {
                    item {
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Button(
                                onClick = { pullOpen = true; pullOut = null },
                                shape = RoundedCornerShape(11.dp),
                                contentPadding = PaddingValues(horizontal = 14.dp, vertical = 6.dp),
                            ) {
                                Icon(Icons.Outlined.Download, null, modifier = Modifier.size(15.dp))
                                Spacer(Modifier.width(6.dp))
                                Text("拉取镜像", fontSize = 13.sp, fontWeight = FontWeight.SemiBold)
                            }
                            OutlinedButton(
                                enabled = !busy,
                                onClick = { act { pullOut = "清理完成\n" + Api.dockerPrune(clientFor(server)) } },
                                shape = RoundedCornerShape(11.dp),
                                contentPadding = PaddingValues(horizontal = 14.dp, vertical = 6.dp),
                            ) {
                                Icon(Icons.Outlined.CleaningServices, null, modifier = Modifier.size(15.dp))
                                Spacer(Modifier.width(6.dp))
                                Text("清理悬空镜像", fontSize = 13.sp)
                            }
                        }
                    }
                    pullOut?.let { out ->
                        item { CodeBlock(out.take(2500), height = 120.dp) }
                    }
                    val q = query.trim().lowercase()
                    val list = images?.filter { i ->
                        q.isBlank() || jstr(i, "Repository").lowercase().contains(q) || jstr(i, "Tag").lowercase().contains(q)
                    }
                    when {
                        list == null -> item { LoadingView() }
                        list.isEmpty() -> item { EmptyView(if (query.isBlank()) "没有镜像" else "没有匹配的镜像") }
                        else -> items(list) { i -> ImageCard(i) }
                    }
                }
            }
        }
    }

    logs?.let { (name, content) ->
        AlertDialog(
            onDismissRequest = { logs = null },
            title = { Text("日志 · $name") },
            text = { CodeBlock(content.take(6000), height = 360.dp) },
            confirmButton = { TextButton(onClick = { logs = null }) { Text("关闭") } },
        )
    }
    if (pullOpen) {
        AlertDialog(
            onDismissRequest = { pullOpen = false },
            title = { Text("拉取镜像") },
            text = {
                Column {
                    OutlinedTextField(value = pullImage, onValueChange = { pullImage = it }, label = { Text("镜像名，如 nginx:latest") }, modifier = Modifier.fillMaxWidth())
                    Spacer(Modifier.height(8.dp))
                    pullOut?.let { CodeBlock(it.take(3000), height = 200.dp) }
                }
            },
            confirmButton = {
                TextButton(enabled = pullImage.isNotBlank() && !busy, onClick = {
                    act {
                        val (ok, out) = Api.dockerPull(clientFor(server), pullImage.trim())
                        pullOut = (if (ok) "拉取成功\n" else "拉取失败\n") + out.take(2500)
                        if (ok) pullImage = ""
                    }
                }) { Text("拉取") }
            },
            dismissButton = { TextButton(onClick = { pullOpen = false }) { Text("关闭") } },
        )
    }
}

/** 概览小统计块：数值 + 标签 + 状态点。 */
@Composable
private fun StatTile(label: String, value: String, tint: Color, modifier: Modifier = Modifier) {
    Card(
        modifier = modifier,
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
    ) {
        Column(Modifier.padding(horizontal = 14.dp, vertical = 12.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                StatusDot(tint, 7.dp)
                Spacer(Modifier.width(6.dp))
                Text(label, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Spacer(Modifier.height(4.dp))
            Text(value, fontSize = 22.sp, fontWeight = FontWeight.Bold, color = tint)
        }
    }
}

/** 容器卡：状态点 + 名称 + 状态胶囊 + 镜像 + 端口胶囊 + 操作行。 */
@Composable
private fun ContainerCard(
    m: Map<String, JsonElement>,
    server: ServerInfo,
    busy: Boolean,
    onViewLogs: (String) -> Unit,
    onAction: (suspend () -> Unit) -> Unit,
) {
    val name = jstr(m, "Names").removePrefix("/")
    val state = jstr(m, "State")
    val running = state == "running"
    val stColor = if (running) OkGreen else IdleGray
    val ports = jstr(m, "Ports").split(',').map { it.trim() }.filter { it.isNotBlank() }
    Card(
        shape = RoundedCornerShape(16.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        modifier = Modifier.fillMaxWidth().clickable { onViewLogs(name) },
    ) {
        Column(Modifier.padding(14.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                StatusDot(stColor, 9.dp)
                Spacer(Modifier.width(8.dp))
                Text(
                    name, fontWeight = FontWeight.SemiBold, fontSize = 15.sp,
                    modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis,
                )
                TagChip(if (running) "运行中" else state.ifBlank { "未知" }, stColor)
            }
            Spacer(Modifier.height(7.dp))
            Text(
                jstr(m, "Image"),
                fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, fontFamily = FontFamily.Monospace,
                maxLines = 1, overflow = TextOverflow.Ellipsis,
            )
            if (ports.isNotEmpty()) {
                Spacer(Modifier.height(7.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                    ports.take(2).forEach { p -> TagChip(p, SkyBlue, modifier = Modifier.weight(1f, fill = false)) }
                    if (ports.size > 2) TagChip("+${ports.size - 2}", IdleGray)
                }
            }
            Spacer(Modifier.height(10.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                if (running) {
                    ActionChip("停止", busy, icon = Icons.Outlined.StopCircle) { onAction { Api.dockerContainerAction(clientFor(server), name, "stop") } }
                    ActionChip("重启", busy, icon = Icons.Outlined.RestartAlt) { onAction { Api.dockerContainerAction(clientFor(server), name, "restart") } }
                } else {
                    ActionChip("启动", busy, icon = Icons.Outlined.PlayCircle) { onAction { Api.dockerContainerAction(clientFor(server), name, "start") } }
                }
                Spacer(Modifier.weight(1f))
                ActionChip("日志", busy) { onViewLogs(name) }
                ActionChip("删除", busy, danger = true, icon = Icons.Outlined.Delete) {
                    onAction { Api.dockerContainerAction(clientFor(server), name, "remove", force = true) }
                }
            }
        }
    }
}

/** 镜像卡：仓库名（mono）+ tag/大小胶囊。 */
@Composable
private fun ImageCard(m: Map<String, JsonElement>) {
    Card(
        shape = RoundedCornerShape(16.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        modifier = Modifier.fillMaxWidth(),
    ) {
        Column(Modifier.padding(14.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Box(
                    Modifier.size(30.dp).background(SkyBlue.copy(alpha = 0.14f), RoundedCornerShape(9.dp)),
                    contentAlignment = Alignment.Center,
                ) {
                    Icon(Icons.Outlined.ViewInAr, null, tint = SkyBlue, modifier = Modifier.size(16.dp))
                }
                Spacer(Modifier.width(10.dp))
                Text(
                    jstr(m, "Repository").ifBlank { "<none>" },
                    fontWeight = FontWeight.SemiBold, fontSize = 13.sp, fontFamily = FontFamily.Monospace,
                    modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis,
                )
            }
            Spacer(Modifier.height(8.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
                TagChip("tag ${jstr(m, "Tag").ifBlank { "-" }}", SkyBlue)
                TagChip(jstr(m, "Size").ifBlank { "-" }, Emerald)
            }
        }
    }
}

/** 小操作按钮（卡片内）：busy 时禁用防重复点击。 */
@Composable
fun ActionChip(label: String, busy: Boolean, danger: Boolean = false, icon: ImageVector? = null, action: () -> Unit) {
    OutlinedButton(
        enabled = !busy,
        onClick = action,
        shape = RoundedCornerShape(9.dp),
        contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp),
    ) {
        if (icon != null) {
            Icon(
                icon, null, modifier = Modifier.size(13.dp),
                tint = if (danger) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary,
            )
            Spacer(Modifier.width(4.dp))
        }
        Text(label, fontSize = 12.sp, color = if (danger) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary)
    }
}
