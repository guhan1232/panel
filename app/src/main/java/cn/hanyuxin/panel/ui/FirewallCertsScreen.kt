// FirewallCertsScreen.kt —— 防火墙（状态/规则增删/同步/防扫描）+ SSL 证书（列表/删除/上传 PEM/HTTP-01 申请）。
package cn.hanyuxin.panel.ui

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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.Api
import cn.hanyuxin.panel.data.CertInfo
import cn.hanyuxin.panel.data.FirewallRule
import cn.hanyuxin.panel.data.FirewallStatus
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.clientFor
import kotlinx.coroutines.launch

// ============ 防火墙 ============

@Composable
fun FirewallScreen(server: ServerInfo) {
    val scope = rememberCoroutineScope()
    var status by remember { mutableStateOf<FirewallStatus?>(null) }
    var rules by remember { mutableStateOf<List<FirewallRule>?>(null) }
    var reload by remember { mutableStateOf(0) }
    var err by remember { mutableStateOf<String?>(null) }
    var busy by remember { mutableStateOf(false) }
    var addOpen by remember { mutableStateOf(false) }
    var confirmDel by remember { mutableStateOf<FirewallRule?>(null) }
    var proto by remember { mutableStateOf("tcp") }
    var ports by remember { mutableStateOf("") }
    var action by remember { mutableStateOf("accept") }
    var source by remember { mutableStateOf("") }
    var remark by remember { mutableStateOf("") }

    fun act(block: suspend () -> Unit) {
        if (busy) return
        busy = true
        scope.launch {
            try { block(); reload++ } catch (e: Exception) { err = e.message ?: "操作失败" } finally { busy = false }
        }
    }

    LaunchedEffect(server.host, reload) {
        apiGuard(onError = { err = it.message }) { Api.fwStatus(clientFor(server)) }?.let { status = it }
        apiGuard(onError = { err = it.message }) { Api.fwRules(clientFor(server)) }?.let { rules = it }
    }

    Column(Modifier.fillMaxSize().padding(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("防火墙", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            IconButton(onClick = { reload++ }) { Icon(Icons.Outlined.Refresh, "刷新") }
        }
        status?.let { s ->
            PanelCard(Modifier.padding(bottom = 10.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) {
                        Text("防火墙（${s.tool}）", fontWeight = FontWeight.SemiBold)
                        Text(
                            if (s.enabled) "已启用" else "未启用",
                            fontSize = 12.sp, color = if (s.enabled) Color(0xFF22C55E) else Color(0xFF9CA3AF),
                        )
                    }
                    Switch(checked = s.enabled, enabled = false, onCheckedChange = null) // 开关走系统工具安装/卸载，APP 只读展示
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Column(Modifier.weight(1f)) { Text("防扫描", fontSize = 13.sp) }
                    Switch(checked = s.anti_scan, onCheckedChange = { act { Api.fwAntiScan(clientFor(server), it) } })
                }
            }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(bottom = 8.dp)) {
            ActionChip("添加规则", busy) { addOpen = true; ports = ""; source = ""; remark = "" }
            ActionChip("同步到系统", busy) { act { Api.fwSync(clientFor(server)) } }
        }
        ErrorBanner(err)
        val list = rules
        when {
            list == null -> LoadingView()
            list.isEmpty() -> EmptyView("没有规则")
            else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(list, key = { it.id }) { r ->
                    Card(shape = RoundedCornerShape(12.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)) {
                        Row(Modifier.padding(horizontal = 14.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                                    Text("${r.protocol.uppercase()} ${r.ports}", fontWeight = FontWeight.SemiBold, fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace)
                                    Text(
                                        if (r.action == "accept") "放行" else "拒绝",
                                        fontSize = 11.sp,
                                        color = if (r.action == "accept") Color(0xFF22C55E) else MaterialTheme.colorScheme.error,
                                    )
                                }
                                Text(
                                    listOfNotNull(r.source.takeIf { it.isNotBlank() }, r.remark.takeIf { it.isNotBlank() }).joinToString(" · ").ifBlank { "入站" },
                                    fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                            ActionChip("删除", busy, danger = true) { confirmDel = r }
                        }
                    }
                }
            }
        }
    }

    if (addOpen) {
        AlertDialog(
            onDismissRequest = { addOpen = false },
            title = { Text("添加规则") },
            text = {
                Column {
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        listOf("tcp", "udp").forEach { p -> FilterSel(proto == p, onClick = { proto = p }) { Text(p.uppercase()) } }
                        Spacer(Modifier.weight(1f))
                        listOf("accept" to "放行", "deny" to "拒绝").forEach { (k, l) -> FilterSel(action == k, onClick = { action = k }) { Text(l) } }
                    }
                    Spacer(Modifier.height(8.dp))
                    OutlinedTextField(value = ports, onValueChange = { ports = it }, label = { Text("端口（如 80,443 或 8000-9000）") }, modifier = Modifier.fillMaxWidth())
                    OutlinedTextField(value = source, onValueChange = { source = it }, label = { Text("来源 IP/CIDR（可空）") }, modifier = Modifier.fillMaxWidth())
                    OutlinedTextField(value = remark, onValueChange = { remark = it }, label = { Text("备注（可空）") }, modifier = Modifier.fillMaxWidth())
                }
            },
            confirmButton = {
                TextButton(enabled = ports.isNotBlank() && !busy, onClick = {
                    act {
                        Api.fwAddRule(clientFor(server), proto, ports.trim(), action, source.trim(), remark.trim())
                        addOpen = false
                    }
                }) { Text("添加") }
            },
            dismissButton = { TextButton(onClick = { addOpen = false }) { Text("取消") } },
        )
    }
    confirmDel?.let { r ->
        AlertDialog(
            onDismissRequest = { confirmDel = null },
            title = { Text("删除规则") },
            text = { Text("删除 ${r.protocol.uppercase()} ${r.ports}？同步前系统防火墙规则保持不变。") },
            confirmButton = {
                TextButton(onClick = { act { Api.fwDeleteRule(clientFor(server), r.id); confirmDel = null } }) { Text("删除", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { confirmDel = null }) { Text("取消") } },
        )
    }
}

/** 双态选择小胶囊（FilterChip 简化封装）。 */
@Composable
private fun FilterSel(selected: Boolean, onClick: () -> Unit, label: @Composable () -> Unit) {
    androidx.compose.material3.FilterChip(selected = selected, onClick = onClick, label = label)
}

// ============ SSL 证书 ============

@Composable
fun CertsScreen(server: ServerInfo) {
    val scope = rememberCoroutineScope()
    var certs by remember { mutableStateOf<List<CertInfo>?>(null) }
    var reload by remember { mutableStateOf(0) }
    var err by remember { mutableStateOf<String?>(null) }
    var busy by remember { mutableStateOf(false) }
    var issueOpen by remember { mutableStateOf(false) }
    var domains by remember { mutableStateOf("") }
    var email by remember { mutableStateOf("") }
    var issueOut by remember { mutableStateOf<String?>(null) }
    var confirmDel by remember { mutableStateOf<CertInfo?>(null) }

    fun act(block: suspend () -> Unit) {
        if (busy) return
        busy = true
        scope.launch {
            try { block(); reload++ } catch (e: Exception) { err = e.message ?: "操作失败" } finally { busy = false }
        }
    }

    LaunchedEffect(server.host, reload) {
        apiGuard(onError = { err = it.message }) { Api.certs(clientFor(server)) }?.let { certs = it }
    }

    Column(Modifier.fillMaxSize().padding(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("SSL 证书", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            IconButton(onClick = { reload++ }) { Icon(Icons.Outlined.Refresh, "刷新") }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(bottom = 8.dp)) {
            ActionChip("申请证书", busy) { issueOpen = true; issueOut = null }
        }
        Text("Let's Encrypt HTTP-01 验证；证书剩余 <30 天面板自动续期", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(bottom = 8.dp))
        ErrorBanner(err)
        val list = certs
        when {
            list == null -> LoadingView()
            list.isEmpty() -> EmptyView("没有证书")
            else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(list, key = { it.domain }) { c ->
                    Card(shape = RoundedCornerShape(12.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)) {
                        Row(Modifier.padding(horizontal = 14.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text(c.domain, fontWeight = FontWeight.SemiBold, fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace)
                                Text(
                                    "到期 ${c.not_after.take(10)} · 剩 ${c.days_left} 天 · ${if (c.source == "acme") "ACME" else "上传"}",
                                    fontSize = 11.sp,
                                    color = when {
                                        c.days_left <= 30 -> MaterialTheme.colorScheme.error
                                        c.days_left <= 60 -> Color(0xFFF59E0B)
                                        else -> MaterialTheme.colorScheme.onSurfaceVariant
                                    },
                                )
                            }
                            ActionChip("删除", busy, danger = true) { confirmDel = c }
                        }
                    }
                }
            }
        }
    }

    if (issueOpen) {
        AlertDialog(
            onDismissRequest = { issueOpen = false },
            title = { Text("申请证书（HTTP-01）") },
            text = {
                Column {
                    OutlinedTextField(value = domains, onValueChange = { domains = it }, label = { Text("域名（多个用逗号分隔）") }, modifier = Modifier.fillMaxWidth())
                    Spacer(Modifier.height(6.dp))
                    OutlinedTextField(value = email, onValueChange = { email = it }, label = { Text("邮箱") }, modifier = Modifier.fillMaxWidth())
                    Spacer(Modifier.height(8.dp))
                    issueOut?.let { Text(it.take(2500), fontSize = 11.sp, fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace, modifier = Modifier.height(180.dp)) }
                    Text("域名需已解析到本服务器且 80 端口可访问", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            },
            confirmButton = {
                TextButton(enabled = domains.isNotBlank() && email.contains("@") && !busy, onClick = {
                    act {
                        val list = domains.split(",", "，").map { it.trim() }.filter { it.isNotBlank() }
                        val d = Api.certIssue(clientFor(server), list, email.trim())
                        issueOut = "签发成功：${d["domain"]}\n证书目录：${d["dir"]}"
                        domains = ""; email = ""
                    }
                }) { Text("申请") }
            },
            dismissButton = { TextButton(onClick = { issueOpen = false }) { Text("关闭") } },
        )
    }
    confirmDel?.let { c ->
        AlertDialog(
            onDismissRequest = { confirmDel = null },
            title = { Text("删除证书") },
            text = { Text("删除 ${c.domain}？已引用它的站点 HTTPS 将失效。") },
            confirmButton = {
                TextButton(onClick = { act { Api.certDelete(clientFor(server), c.domain); confirmDel = null } }) { Text("删除", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { confirmDel = null }) { Text("取消") } },
        )
    }
}
