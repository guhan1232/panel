// ProcNodesScreen.kt —— 进程管理（列表/kill + systemd 服务控制）与节点管理（状态/远程执行）。
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
import cn.hanyuxin.panel.data.NodeItem
import cn.hanyuxin.panel.data.ProcInfo
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.ServiceInfo
import cn.hanyuxin.panel.data.clientFor
import kotlinx.coroutines.launch

// ============ 进程管理 ============

@Composable
fun ProcScreen(server: ServerInfo) {
    val scope = rememberCoroutineScope()
    var sort by remember { mutableStateOf("cpu") }
    var procs by remember { mutableStateOf<List<ProcInfo>?>(null) }
    var services by remember { mutableStateOf<List<ServiceInfo>?>(null) }
    var reload by remember { mutableStateOf(0) }
    var err by remember { mutableStateOf<String?>(null) }
    var busy by remember { mutableStateOf(false) }
    var tab by remember { mutableStateOf("进程") }
    var killTarget by remember { mutableStateOf<ProcInfo?>(null) }
    var svcTarget by remember { mutableStateOf<Pair<ServiceInfo, String>?>(null) }

    fun act(block: suspend () -> Unit) {
        if (busy) return
        busy = true
        scope.launch {
            try { block(); reload++ } catch (e: Exception) { err = e.message ?: "操作失败" } finally { busy = false }
        }
    }

    LaunchedEffect(server.host, reload, tab, sort) {
        if (tab == "进程") {
            apiGuard(onError = { err = it.message }) { Api.procList(clientFor(server), sort) }?.let { procs = it }
        } else {
            apiGuard(onError = { err = it.message }) { Api.procServices(clientFor(server)) }?.let { services = it }
        }
    }

    Column(Modifier.fillMaxSize().padding(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("进程管理", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            IconButton(onClick = { reload++ }) { Icon(Icons.Outlined.Refresh, "刷新") }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            listOf("进程", "服务").forEach { FilterChip(selected = tab == it, onClick = { tab = it }, label = { Text(it) }) }
            if (tab == "进程") {
                listOf("cpu" to "按CPU", "mem" to "按内存").forEach { (k, l) ->
                    FilterChip(selected = sort == k, onClick = { sort = k }, label = { Text(l) })
                }
            }
        }
        Spacer(Modifier.height(8.dp))
        ErrorBanner(err)
        if (tab == "进程") {
            val list = procs
            when {
                list == null -> LoadingView()
                list.isEmpty() -> EmptyView("无进程数据")
                else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    items(list, key = { it.pid }) { p ->
                        Card(shape = RoundedCornerShape(12.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)) {
                            Row(Modifier.padding(horizontal = 14.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                                Column(Modifier.weight(1f)) {
                                    Text("${p.name} (${p.pid})", fontWeight = FontWeight.SemiBold, fontFamily = FontFamily.Monospace, fontSize = 13.sp)
                                    Text("CPU ${pct(p.cpu)} · 内存 ${pct(p.mem_pct.toDouble())}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                                ActionChip("结束", busy, danger = true) { killTarget = p }
                            }
                        }
                    }
                }
            }
        } else {
            val list = services
            when {
                list == null -> LoadingView()
                list.isEmpty() -> EmptyView("无服务数据")
                else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    items(list) { s ->
                        Card(shape = RoundedCornerShape(12.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)) {
                            Column(Modifier.padding(horizontal = 14.dp, vertical = 10.dp)) {
                                Row(verticalAlignment = Alignment.CenterVertically) {
                                    Column(Modifier.weight(1f)) {
                                        Text(s.unit, fontWeight = FontWeight.SemiBold, fontFamily = FontFamily.Monospace, fontSize = 13.sp)
                                        Text("${s.active}/${s.sub} · ${s.enabled}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                    }
                                    val running = s.active == "active"
                                    ActionChip(if (running) "停止" else "启动", busy) {
                                        svcTarget = s to if (running) "stop" else "start"
                                    }
                                    ActionChip("重启", busy) { svcTarget = s to "restart" }
                                }
                                if (s.description.isNotBlank()) {
                                    Text(s.description, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    killTarget?.let { p ->
        AlertDialog(
            onDismissRequest = { killTarget = null },
            title = { Text("结束进程") },
            text = { Text("结束 ${p.name} (PID ${p.pid})？服务类进程可能被 systemd 自动拉起。") },
            confirmButton = { TextButton(onClick = { act { Api.procKill(clientFor(server), p.pid, false); killTarget = null } }) { Text("结束", color = MaterialTheme.colorScheme.error) } },
            dismissButton = { TextButton(onClick = { killTarget = null }) { Text("取消") } },
        )
    }
    svcTarget?.let { (s, action) ->
        AlertDialog(
            onDismissRequest = { svcTarget = null },
            title = { Text("服务操作") },
            text = { Text("对 ${s.unit} 执行 $action？") },
            confirmButton = {
                TextButton(onClick = { act { Api.procServiceAction(clientFor(server), s.unit, action); svcTarget = null } }) { Text("确定") }
            },
            dismissButton = { TextButton(onClick = { svcTarget = null }) { Text("取消") } },
        )
    }
}

// ============ 节点管理 ============

@Composable
fun NodesScreen(server: ServerInfo) {
    val scope = rememberCoroutineScope()
    var nodes by remember { mutableStateOf<List<NodeItem>?>(null) }
    var reload by remember { mutableStateOf(0) }
    var err by remember { mutableStateOf<String?>(null) }
    var busy by remember { mutableStateOf(false) }
    var execTarget by remember { mutableStateOf<NodeItem?>(null) }
    var execCmd by remember { mutableStateOf("") }
    var execOut by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(server.host, reload) {
        apiGuard(onError = { err = it.message }) { Api.nodes(clientFor(server)) }?.let { nodes = it }
    }

    Column(Modifier.fillMaxSize().padding(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("节点管理", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            IconButton(onClick = { reload++ }) { Icon(Icons.Outlined.Refresh, "刷新") }
        }
        Spacer(Modifier.height(8.dp))
        ErrorBanner(err)
        val list = nodes
        when {
            list == null -> LoadingView()
            list.isEmpty() -> EmptyView("没有节点；新增节点请用 Web 端")
            else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                items(list, key = { it.id }) { n ->
                    Card(shape = RoundedCornerShape(14.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)) {
                        Column(Modifier.padding(14.dp)) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Text(n.name.ifBlank { n.hostname }, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
                                Text(
                                    if (n.last_seen.isNotBlank()) "已上报" else "未上报",
                                    fontSize = 12.sp,
                                    color = if (n.last_seen.isNotBlank()) Color(0xFF22C55E) else Color(0xFF9CA3AF),
                                )
                            }
                            Text("${n.os} ${n.arch} · ${n.ip}", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            if (n.last_seen.isNotBlank()) {
                                Text("最近上报 ${n.last_seen.take(19)}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                            Spacer(Modifier.height(8.dp))
                            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                                ActionChip("执行脚本", busy) { execTarget = n; execOut = null; execCmd = "" }
                            }
                        }
                    }
                }
            }
        }
    }

    execTarget?.let { n ->
        AlertDialog(
            onDismissRequest = { execTarget = null },
            title = { Text("节点执行 · ${n.name.ifBlank { n.hostname }}") },
            text = {
                Column {
                    OutlinedTextField(value = execCmd, onValueChange = { execCmd = it }, label = { Text("shell 脚本") }, modifier = Modifier.fillMaxWidth(), minLines = 3)
                    Spacer(Modifier.height(8.dp))
                    execOut?.let { Text(it.take(4000), fontSize = 11.sp, fontFamily = FontFamily.Monospace, modifier = Modifier.height(220.dp)) }
                }
            },
            confirmButton = {
                TextButton(enabled = execCmd.isNotBlank() && !busy, onClick = {
                    busy = true
                    scope.launch {
                        try {
                            execOut = Api.nodeExec(clientFor(server), n.id, execCmd)
                            execCmd = ""
                        } catch (e: Exception) {
                            execOut = "执行失败: ${e.message}"
                        } finally { busy = false }
                    }
                }) { Text("执行") }
            },
            dismissButton = { TextButton(onClick = { execTarget = null }) { Text("关闭") } },
        )
    }
}
