// DockerScreen.kt —— Docker 管理：状态概览 + 容器（启停/重启/删除/日志）+ 镜像（列表/拉取/清理）。
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
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
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

    LaunchedEffect(server.host, reload, tab) {
        apiGuard(onError = { err = it.message }) { Api.dockerOverview(clientFor(server)) }?.let { overview = it }
        if (tab == "容器") {
            apiGuard(onError = { err = it.message }) { Api.dockerContainers(clientFor(server)) }?.let { containers = it }
        } else {
            apiGuard(onError = { err = it.message }) { Api.dockerImages(clientFor(server)) }?.let { images = it }
        }
    }

    Column(Modifier.fillMaxSize().padding(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("Docker", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            IconButton(onClick = { reload++ }) { Icon(Icons.Outlined.Refresh, "刷新") }
        }
        overview?.let { o ->
            if (o.installed) {
                Text("运行 ${o.running} · 已停止 ${o.stopped} · 镜像 ${o.images}", fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            } else {
                Text("Docker 未安装，请在软件商店安装", fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        Spacer(Modifier.height(8.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            listOf("容器", "镜像").forEach { FilterChip(selected = tab == it, onClick = { tab = it }, label = { Text(it) }) }
        }
        Spacer(Modifier.height(8.dp))
        ErrorBanner(err)

        if (tab == "容器") {
            val list = containers
            when {
                list == null -> LoadingView()
                list.isEmpty() -> EmptyView("没有容器")
                else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    items(list) { m ->
                        val name = jstr(m, "Names").removePrefix("/")
                        val state = jstr(m, "State")
                        val stColor = if (state == "running") Color(0xFF22C55E) else Color(0xFF9CA3AF)
                        Card(
                            shape = RoundedCornerShape(14.dp),
                            colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                            modifier = Modifier.fillMaxWidth().clickable {
                                act {
                                    apiGuard(onError = { err = it.message }) { Api.dockerLogs(clientFor(server), name) }
                                        ?.let { logs = name to it }
                                }
                            },
                        ) {
                            Column(Modifier.padding(14.dp)) {
                                Row(verticalAlignment = Alignment.CenterVertically) {
                                    Text(name, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
                                    Text(state, fontSize = 12.sp, color = stColor, fontWeight = FontWeight.Medium)
                                }
                                Text(jstr(m, "Image"), fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, fontFamily = FontFamily.Monospace)
                                if (jstr(m, "Ports").isNotBlank()) {
                                    Text(jstr(m, "Ports").take(80), fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                                Spacer(Modifier.height(8.dp))
                                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                                    if (state == "running") {
                                        ActionChip("停止", busy) { act { Api.dockerContainerAction(clientFor(server), name, "stop") } }
                                        ActionChip("重启", busy) { act { Api.dockerContainerAction(clientFor(server), name, "restart") } }
                                    } else {
                                        ActionChip("启动", busy) { act { Api.dockerContainerAction(clientFor(server), name, "start") } }
                                    }
                                    ActionChip("删除", busy, danger = true) { act { Api.dockerContainerAction(clientFor(server), name, "remove", force = true) } }
                                }
                            }
                        }
                    }
                }
            }
        } else {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(bottom = 8.dp)) {
                OutlinedButton(onClick = { pullOpen = true; pullOut = null }) { Text("拉取镜像") }
                OutlinedButton(enabled = !busy, onClick = { act { pullOut = "清理完成\n" + Api.dockerPrune(clientFor(server)) } }) { Text("清理悬空镜像") }
            }
            pullOut?.let {
                Text(it.take(2500), fontSize = 11.sp, fontFamily = FontFamily.Monospace, modifier = Modifier.fillMaxWidth().height(120.dp).padding(bottom = 8.dp))
            }
            val list = images
            when {
                list == null -> LoadingView()
                list.isEmpty() -> EmptyView("没有镜像")
                else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                    items(list) { m ->
                        Card(shape = RoundedCornerShape(14.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)) {
                            Column(Modifier.padding(14.dp)) {
                                Text(jstr(m, "Repository").ifBlank { "<none>" }, fontWeight = FontWeight.SemiBold, fontFamily = FontFamily.Monospace)
                                Text("tag=${jstr(m, "Tag")}  size=${jstr(m, "Size")}", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                        }
                    }
                }
            }
        }
    }

    logs?.let { (name, content) ->
        AlertDialog(
            onDismissRequest = { logs = null },
            title = { Text("日志 · $name") },
            text = { Text(content.take(6000), fontSize = 11.sp, fontFamily = FontFamily.Monospace, modifier = Modifier.fillMaxWidth().height(360.dp)) },
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
                    pullOut?.let { Text(it.take(3000), fontSize = 11.sp, fontFamily = FontFamily.Monospace, modifier = Modifier.height(200.dp)) }
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

/** 小操作按钮（卡片内）：busy 时禁用防重复点击。 */
@Composable
fun ActionChip(label: String, busy: Boolean, danger: Boolean = false, action: () -> Unit) {
    OutlinedButton(
        enabled = !busy,
        onClick = action,
        contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 10.dp, vertical = 4.dp),
    ) {
        Text(label, fontSize = 12.sp, color = if (danger) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.primary)
    }
}
