// DashboardScreen.kt —— 概览（宝塔式）：服务器头卡（名称/运行徽章/IP/上下行网速）
// + 指标网格（负载竖条/CPU 圆弧/内存圆弧/磁盘水平条）+ 趋势图表卡。
package cn.hanyuxin.panel.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ArrowDownward
import androidx.compose.material.icons.outlined.ArrowUpward
import androidx.compose.material.icons.outlined.Dns
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.Api
import cn.hanyuxin.panel.data.DashboardSummary
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.ServerStore
import cn.hanyuxin.panel.data.clientFor
import kotlinx.coroutines.delay

/** 主色（青绿）；OkGreen 复用 MeScreens.kt 同包定义。 */
private val BrandTeal = Color(0xFF0D9488)

/** 绿色小徽章（运行天数 / 核数 / 容量等）。 */
@Composable
private fun MiniBadge(text: String) {
    Text(
        text,
        fontSize = 11.sp, fontWeight = FontWeight.Bold,
        color = Color.White,
        modifier = Modifier
            .background(OkGreen, RoundedCornerShape(5.dp))
            .padding(horizontal = 6.dp, vertical = 2.dp),
    )
}

/** 浅底状态 pill（流畅/繁忙等）。 */
@Composable
private fun StatePill(text: String, ok: Boolean) {
    Text(
        text,
        fontSize = 11.sp, fontWeight = FontWeight.Bold,
        color = if (ok) OkGreen else Color(0xFFF59E0B),
        modifier = Modifier
            .background((if (ok) OkGreen else Color(0xFFF59E0B)).copy(alpha = 0.12f), RoundedCornerShape(5.dp))
            .padding(horizontal = 6.dp, vertical = 2.dp),
    )
}

/** 圆弧仪表：360° 底环 + 进度弧 + 中央数值。 */
@Composable
private fun GaugeArc(progress: Float, centerText: String, tint: Color, modifier: Modifier = Modifier) {
    Box(modifier.size(76.dp), contentAlignment = Alignment.Center) {
        Canvas(Modifier.size(76.dp)) {
            val stroke = 9.dp.toPx()
            drawArc(
                Color(0xFFE8ECEF), startAngle = 0f, sweepAngle = 360f, useCenter = false,
                style = Stroke(stroke, cap = StrokeCap.Round),
            )
            drawArc(
                tint, startAngle = -90f, sweepAngle = 360f * progress.coerceIn(0f, 1f), useCenter = false,
                style = Stroke(stroke, cap = StrokeCap.Round),
            )
        }
        Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Text(centerText, fontSize = 15.sp, fontWeight = FontWeight.Bold, color = tint)
        }
    }
}

/** 负载竖条仪表（圆角矩形 + 底部进度填充 + 百分比文字；低负载保底可见高度）。 */
@Composable
private fun LoadBar(progress: Float, modifier: Modifier = Modifier) {
    val p = progress.coerceIn(0f, 1f)
    Box(
        modifier
            .width(72.dp)
            .height(96.dp)
            .background(Color(0xFFE8ECEF), RoundedCornerShape(10.dp)),
        contentAlignment = Alignment.BottomCenter,
    ) {
        Box(
            Modifier
                .fillMaxWidth()
                .height(maxOf(8.dp, (96 * p).dp))
                .background(BrandTeal, RoundedCornerShape(topStart = 10.dp, topEnd = 10.dp)),
        )
        Text(
            "${(p * 100).toInt()}%",
            fontSize = 13.sp, fontWeight = FontWeight.Bold,
            color = if (p > 0.35f) Color.White else BrandTeal,
            modifier = Modifier.padding(bottom = 8.dp),
        )
    }
}

/** 水平进度条（磁盘用）。 */
@Composable
private fun HBar(progress: Float, tint: Color, modifier: Modifier = Modifier) {
    Box(
        modifier
            .fillMaxWidth()
            .height(10.dp)
            .background(Color(0xFFE8ECEF), RoundedCornerShape(5.dp)),
    ) {
        Box(
            Modifier
                .fillMaxWidth(progress.coerceIn(0f, 1f))
                .height(10.dp)
                .background(tint, RoundedCornerShape(5.dp)),
        )
    }
}

@Composable
fun DashboardScreen(server: ServerInfo) {
    var data by remember(server.host) { mutableStateOf<DashboardSummary?>(null) }
    var err by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(server.host) {
        while (true) {
            runCatching { Api.dashboard(clientFor(server)) }
                .onSuccess { data = it; err = null }
                .onFailure { err = (it::class.simpleName ?: "Error") + ": " + (it.message ?: "无详情") }
            delay(3000)
        }
    }

    val d = data
    val cur = d?.current
    val loadPct = ((cur?.load1 ?: 0.0) / 4.0).coerceIn(0.0, 1.0).toFloat() // 负载 / 4 核归一
    val disk = d?.disks?.firstOrNull()
    val uptimeDays = (d?.host?.uptime ?: 0) / 86400

    Column(Modifier.fillMaxSize().background(Color(0xFFF4F6FA))) {
        LazyColumn(
            Modifier.fillMaxSize().padding(horizontal = 14.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            // ---- 标题 ----
            item {
                Spacer(Modifier.height(6.dp))
                Text("概览", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
            }
            if (d == null) {
                item { LoadingView(); ErrorBanner(err) }
                return@LazyColumn
            }
            item { ErrorBanner(err) }

            // ---- 头卡：图标 + 主机名 + 右侧上下行网速 ----
            item {
                PanelCard {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(
                            Modifier
                                .size(46.dp)
                                .background(BrandTeal.copy(alpha = 0.12f), RoundedCornerShape(12.dp)),
                            contentAlignment = Alignment.Center,
                        ) {
                            Icon(Icons.Outlined.Dns, null, tint = BrandTeal, modifier = Modifier.size(26.dp))
                        }
                        Spacer(Modifier.width(12.dp))
                        Column(Modifier.weight(1f)) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                // 标题用主机名（短、可断），避免 URL 长串把徽章挤成竖条
                                Text(d.host?.hostname ?: server.name.ifBlank { "服务器" }, fontWeight = FontWeight.Bold, fontSize = 16.sp)
                                Spacer(Modifier.width(6.dp))
                                if (uptimeDays > 0) MiniBadge("运行 $uptimeDays 天")
                            }
                            Text(
                                server.host.removePrefix("https://").removePrefix("http://"),
                                fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                            )
                        }
                        Column(horizontalAlignment = Alignment.End) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Icon(Icons.Outlined.ArrowDownward, null, tint = AppColors.Blue, modifier = Modifier.size(14.dp))
                                Text(" " + humanRate(cur?.recv ?: 0), fontSize = 12.sp, fontWeight = FontWeight.Medium)
                            }
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Icon(Icons.Outlined.ArrowUpward, null, tint = AppColors.Amber, modifier = Modifier.size(14.dp))
                                Text(" " + humanRate(cur?.sent ?: 0), fontSize = 12.sp, fontWeight = FontWeight.Medium)
                            }
                        }
                    }
                }
            }

            // ---- 指标网格：负载 / CPU ----
            item {
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    PanelCard(Modifier.weight(1f)) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text("负载", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                            Spacer(Modifier.width(8.dp))
                            StatePill(if (loadPct < 0.5f) "流畅" else if (loadPct < 0.8f) "正常" else "繁忙", loadPct < 0.8f)
                        }
                        Spacer(Modifier.height(10.dp))
                        Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                            LoadBar(loadPct)
                        }
                        Spacer(Modifier.height(6.dp))
                        Text("负载 ${String.format("%.2f", cur?.load1 ?: 0.0)}", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    PanelCard(Modifier.weight(1f)) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text("CPU", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                            Spacer(Modifier.width(8.dp))
                            MiniBadge("在线")
                        }
                        Spacer(Modifier.height(8.dp))
                        Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                            GaugeArc(
                                ((cur?.cpu ?: 0.0) / 100.0).toFloat(),
                                pct(cur?.cpu ?: 0.0), BrandTeal,
                            )
                        }
                    }
                }
            }

            // ---- 指标网格：内存 / 磁盘 ----
            item {
                Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    PanelCard(Modifier.weight(1f)) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text("内存", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                            Spacer(Modifier.width(8.dp))
                            MiniBadge(humanSize(cur?.mem_used ?: 0))
                        }
                        Spacer(Modifier.height(8.dp))
                        Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
                            GaugeArc(
                                ((cur?.mem_pct ?: 0.0) / 100.0).toFloat(),
                                pct(cur?.mem_pct ?: 0.0), AppColors.Blue,
                            )
                        }
                    }
                    PanelCard(Modifier.weight(1f)) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Text(disk?.mount ?: "磁盘", fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
                            Spacer(Modifier.width(8.dp))
                            MiniBadge(humanSize(disk?.total ?: 0))
                        }
                        Spacer(Modifier.height(14.dp))
                        HBar(((disk?.used_pct ?: 0.0) / 100.0).toFloat(), BrandTeal)
                        Spacer(Modifier.height(8.dp))
                        Row {
                            Text(pct(disk?.used_pct ?: 0.0), fontSize = 15.sp, fontWeight = FontWeight.Bold, color = BrandTeal)
                            Spacer(Modifier.weight(1f))
                            Text("${humanSize(disk?.used ?: 0)} / ${humanSize(disk?.total ?: 0)}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                    }
                }
            }

            // ---- 趋势图表 ----
            item {
                PanelCard {
                    Text("CPU 使用率", fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    LineChart(d.history.map { it.cpu }, BrandTeal)
                }
            }
            item {
                PanelCard {
                    Text("内存使用率", fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    LineChart(d.history.map { it.mem_pct }, AppColors.Blue)
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

            // ---- 系统信息 ----
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
            item { Spacer(Modifier.height(4.dp)) }
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
