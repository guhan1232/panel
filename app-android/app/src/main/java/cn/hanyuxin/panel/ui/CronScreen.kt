// CronScreen.kt —— 计划任务：列表 + 启停 + 立即执行 + 删除 + 执行日志 + 新建 shell 任务。
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
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.Api
import cn.hanyuxin.panel.data.PageResp
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.Task
import cn.hanyuxin.panel.data.TaskLog
import cn.hanyuxin.panel.data.clientFor
import kotlinx.coroutines.launch

private val TASK_TYPES = mapOf(
    "shell" to "脚本", "backup_dir" to "目录备份", "visit_url" to "访问URL",
    "backup_db" to "数据库备份", "log_rotate" to "日志轮转", "sys_update" to "系统更新",
)

@Composable
fun CronScreen(server: ServerInfo) {
    val scope = rememberCoroutineScope()
    var tasks by remember { mutableStateOf<List<Task>?>(null) }
    var reload by remember { mutableStateOf(0) }
    var err by remember { mutableStateOf<String?>(null) }
    var busy by remember { mutableStateOf(false) }
    var logs by remember { mutableStateOf<Pair<Task, PageResp<TaskLog>>?>(null) }
    var logDetail by remember { mutableStateOf<TaskLog?>(null) }
    var createOpen by remember { mutableStateOf(false) }
    var nName by remember { mutableStateOf("") }
    var nSpec by remember { mutableStateOf("0 3 * * *") }
    var nCmd by remember { mutableStateOf("") }
    var confirmDel by remember { mutableStateOf<Task?>(null) }

    fun act(block: suspend () -> Unit) {
        if (busy) return
        busy = true
        scope.launch {
            try { block(); reload++ } catch (e: Exception) { err = e.message ?: "操作失败" } finally { busy = false }
        }
    }

    LaunchedEffect(server.host, reload) {
        apiGuard(onError = { err = it.message }) { Api.tasks(clientFor(server)) }?.let { tasks = it }
    }

    Column(Modifier.fillMaxSize().padding(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("计划任务", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            IconButton(onClick = { reload++ }) { Icon(Icons.Outlined.Refresh, "刷新") }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(bottom = 8.dp)) {
            ActionChip("新建脚本任务", busy) { createOpen = true }
        }
        ErrorBanner(err)
        val list = tasks
        when {
            list == null -> LoadingView()
            list.isEmpty() -> EmptyView("没有计划任务")
            else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                items(list, key = { it.id }) { t ->
                    Card(shape = RoundedCornerShape(14.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)) {
                        Column(Modifier.padding(14.dp)) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Column(Modifier.weight(1f).clickable {
                                    act {
                                        apiGuard(onError = { err = it.message }) { Api.taskLogs(clientFor(server), t.id) }?.let { logs = t to it }
                                    }
                                }) {
                                    Text(t.name, fontWeight = FontWeight.SemiBold)
                                    Text(
                                        "${TASK_TYPES[t.type] ?: t.type} · ${t.spec}",
                                        fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, fontFamily = FontFamily.Monospace,
                                    )
                                }
                                Switch(checked = t.enabled, onCheckedChange = { act { Api.taskToggle(clientFor(server), t.id) } })
                            }
                            if (t.last_status.isNotBlank() && t.last_status != "-") {
                                val c = when (t.last_status) {
                                    "success" -> Color(0xFF22C55E); "failed", "timeout" -> MaterialTheme.colorScheme.error
                                    "running" -> Color(0xFFF59E0B); else -> Color.Gray
                                }
                                Text("上次：${t.last_status} ${t.last_run_at?.take(19) ?: ""}", fontSize = 11.sp, color = c, modifier = Modifier.padding(top = 4.dp))
                            }
                            Spacer(Modifier.height(8.dp))
                            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                                ActionChip("执行", busy) { act { Api.taskRun(clientFor(server), t.id) } }
                                ActionChip("删除", busy, danger = true) { confirmDel = t }
                            }
                        }
                    }
                }
            }
        }
    }

    // 执行日志
    logs?.let { (t, page) ->
        AlertDialog(
            onDismissRequest = { logs = null },
            title = { Text("执行日志 · ${t.name}") },
            text = {
                Column(Modifier.height(340.dp)) {
                    if (page.items.isEmpty()) Text("暂无执行记录", color = MaterialTheme.colorScheme.onSurfaceVariant)
                    LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                        items(page.items, key = { it.id }) { l ->
                            Column(Modifier.fillMaxWidth().clickable {
                                act {
                                    apiGuard(onError = { err = it.message }) { Api.taskLogDetail(clientFor(server), l.id) }?.let { logDetail = it }
                                }
                            }) {
                                Row {
                                    val c = when (l.status) {
                                        "success" -> Color(0xFF22C55E); "failed", "timeout" -> MaterialTheme.colorScheme.error
                                        "running" -> Color(0xFFF59E0B); else -> Color.Gray
                                    }
                                    Text(l.status, fontSize = 12.sp, color = c, fontWeight = FontWeight.Medium)
                                    Spacer(Modifier.width(8.dp))
                                    Text("${l.trigger} · ${l.start_at.take(19)} · ${l.cost_ms}ms", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                            }
                        }
                    }
                }
            },
            confirmButton = { TextButton(onClick = { logs = null }) { Text("关闭") } },
        )
    }
    // 日志详情
    logDetail?.let { l ->
        AlertDialog(
            onDismissRequest = { logDetail = null },
            title = { Text("输出 · ${l.status}") },
            text = { Text(l.output.ifBlank { "(无输出)" }.take(5000), fontSize = 11.sp, fontFamily = FontFamily.Monospace, modifier = Modifier.height(320.dp)) },
            confirmButton = { TextButton(onClick = { logDetail = null }) { Text("关闭") } },
        )
    }
    // 新建
    if (createOpen) {
        AlertDialog(
            onDismissRequest = { createOpen = false; nName = ""; nCmd = "" },
            title = { Text("新建脚本任务") },
            text = {
                Column {
                    OutlinedTextField(value = nName, onValueChange = { nName = it }, label = { Text("任务名") }, modifier = Modifier.fillMaxWidth())
                    Spacer(Modifier.height(6.dp))
                    OutlinedTextField(value = nSpec, onValueChange = { nSpec = it }, label = { Text("cron 表达式（分 时 日 月 周）") }, modifier = Modifier.fillMaxWidth())
                    Spacer(Modifier.height(6.dp))
                    OutlinedTextField(value = nCmd, onValueChange = { nCmd = it }, label = { Text("shell 命令") }, modifier = Modifier.fillMaxWidth(), minLines = 3)
                }
            },
            confirmButton = {
                TextButton(enabled = nName.isNotBlank() && nSpec.isNotBlank() && nCmd.isNotBlank() && !busy, onClick = {
                    act { Api.taskCreate(clientFor(server), nName.trim(), nSpec.trim(), nCmd.trim()); createOpen = false; nName = ""; nCmd = "" }
                }) { Text("创建") }
            },
            dismissButton = { TextButton(onClick = { createOpen = false }) { Text("取消") } },
        )
    }
    // 删除确认
    confirmDel?.let { t ->
        AlertDialog(
            onDismissRequest = { confirmDel = null },
            title = { Text("删除任务") },
            text = { Text("将删除任务「${t.name}」及其执行日志。") },
            confirmButton = {
                TextButton(onClick = { act { Api.taskDelete(clientFor(server), t.id); confirmDel = null } }) { Text("删除", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { confirmDel = null }) { Text("取消") } },
        )
    }
}
