// DashboardScreen.kt —— 仪表盘：3s 轮询 /api/dashboard/summary（与 Web 端节奏一致）。
package cn.hanyuxin.panel.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Memory
import androidx.compose.material.icons.outlined.NetworkCheck
import androidx.compose.material.icons.outlined.Speed
import androidx.compose.material.icons.outlined.Storage
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.Api
import cn.hanyuxin.panel.data.DashboardSummary
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.clientFor
import kotlinx.coroutines.delay

@Composable
fun DashboardScreen(server: ServerInfo) {
    var data by remember(server.host) { mutableStateOf<DashboardSummary?>(null) }
    var err by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(server.host) {
        while (true) {
            apiGuard(onError = { err = it.message ?: "请求失败" }) { Api.dashboard(clientFor(server)) }?.let { data = it; err = null }
            delay(3000)
        }
    }

    val d = data
    Column(Modifier.fillMaxSize().padding(16.dp)) {
        Text(
            d?.host?.hostname ?: "加载中…",
            style = MaterialTheme.typography.titleLarge,
            fontWeight = FontWeight.Bold,
        )
        Text(
            listOfNotNull(d?.host?.platform, d?.host?.platform_version, d?.host?.kernel_arch).joinToString(" · "),
            fontSize = 13.sp,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(12.dp))
        if (d == null) {
            LoadingView()
            ErrorBanner(err)
            return
        }
        ErrorBanner(err)
        LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp)) {
            val cur = d.current
            item {
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    StatCard(
                        Icons.Outlined.Speed, "CPU", pct(cur?.cpu ?: 0.0),
                        sub = "负载 ${String.format("%.2f", cur?.load1 ?: 0.0)}",
                        tint = Color(0xFF2DD4BF), modifier = Modifier.weight(1f),
                    )
                    StatCard(
                        Icons.Outlined.Memory, "内存", pct(cur?.mem_pct ?: 0.0),
                        sub = "${humanSize(cur?.mem_used ?: 0)} / ${humanSize(cur?.mem_total ?: 0)}",
                        tint = Color(0xFF60A5FA), modifier = Modifier.weight(1f),
                    )
                }
            }
            item {
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    StatCard(
                        Icons.Outlined.NetworkCheck, "下载", humanRate(cur?.recv ?: 0),
                        sub = "实时接收", tint = Color(0xFF38BDF8), modifier = Modifier.weight(1f),
                    )
                    StatCard(
                        Icons.Outlined.NetworkCheck, "上传", humanRate(cur?.sent ?: 0),
                        sub = "实时发送", tint = Color(0xFFF59E0B), modifier = Modifier.weight(1f),
                    )
                }
            }
            item {
                PanelCard {
                    Text("CPU 使用率", fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    LineChart(d.history.map { it.cpu }, Color(0xFF2DD4BF))
                }
            }
            item {
                PanelCard {
                    Text("内存使用率", fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    LineChart(d.history.map { it.mem_pct }, Color(0xFF60A5FA))
                }
            }
            item {
                PanelCard {
                    Text("网络速率", fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Text("蓝色=下载 · 橙色=上传", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Spacer(Modifier.height(4.dp))
                    NetChart(d.history.map { it.recv.toDouble() }, d.history.map { it.sent.toDouble() })
                }
            }
            items(d.disks.size) { i ->
                val dk = d.disks[i]
                StatCard(
                    Icons.Outlined.Storage, dk.mount, pct(dk.used_pct),
                    sub = "${humanSize(dk.used)} / ${humanSize(dk.total)} · ${dk.fs}",
                    tint = Color(0xFFA78BFA),
                )
            }
            item {
                d.host?.let {
                    PanelCard {
                        KeyValue("主机名", it.hostname, mono = true)
                        KeyValue("系统", listOfNotNull(it.os, it.platform, it.platform_family).joinToString(" "))
                        KeyValue("内核", "${it.kernel_version} ${it.kernel_arch}", mono = true)
                        if (it.uptime > 0) KeyValue("运行时长", humanUptime(it.uptime))
                    }
                }
            }
        }
    }
}

private fun humanUptime(seconds: Long): String {
    val days = seconds / 86400
    val hours = (seconds % 86400) / 3600
    val minutes = (seconds % 3600) / 60
    return when {
        days > 0 -> "$days 天 $hours 小时"
        hours > 0 -> "$hours 小时 $minutes 分钟"
        else -> "$minutes 分钟"
    }
}
