// DashboardScreen.kt —— 概览（宝塔 APP 首页式）：
// 渐变横幅头卡（主机名/运行徽章/IP/上下行网速）
// + 监控大卡 2×2 象限：每象限 = 色点+标题 / 大号百分比主视觉 / 细横条进度 / 副信息（色阶绿→黄→红）
// + 自适应量程趋势图 + 系统信息。
package cn.hanyuxin.panel.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.shape.CircleShape
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.Api
import cn.hanyuxin.panel.data.DashboardSummary
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.ServerStore
import cn.hanyuxin.panel.data.clientFor
import kotlinx.coroutines.delay

private val BrandTeal = Color(0xFF0D9488)
private val BrandEmerald = Color(0xFF10B981)
// OkGreen 复用 MeScreens.kt 同包定义（0xFF22C55E）
private val WarnAmber = Color(0xFFF59E0B)
private val DangerRed = Color(0xFFEF4444)
private val TrackGray = Color(0xFFEDF0F3)
private val Hairline = Color(0xFFEEF0F3)
private val LabelGray = Color(0xFF7A8699)

/** 使用率色阶（宝塔语义）：低=绿 / 中=琥珀 / 高=红。 */
private fun tierColor(p: Float): Color = when {
    p < 0.5f -> OkGreen
    p < 0.8f -> WarnAmber
    else -> DangerRed
}

/** 区块标题：左侧青绿竖条 accent + 标题 + 右侧可选组件。 */
@Composable
private fun SectionTitle(text: String, trailing: (@Composable () -> Unit)? = null) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Box(
            Modifier
                .width(4.dp)
                .height(16.dp)
                .background(BrandTeal, RoundedCornerShape(2.dp)),
        )
        Spacer(Modifier.width(8.dp))
        Text(text, fontSize = 16.sp, fontWeight = FontWeight.Bold)
        Spacer(Modifier.weight(1f))
        trailing?.invoke()
    }
}

/** 浅底状态 pill（实时/流畅等）。 */
@Composable
private fun StatePill(text: String, tint: Color) {
    Text(
        text,
        fontSize = 11.sp, fontWeight = FontWeight.Bold,
        color = tint,
        modifier = Modifier
            .background(tint.copy(alpha = 0.12f), RoundedCornerShape(5.dp))
            .padding(horizontal = 6.dp, vertical = 2.dp),
    )
}

/**
 * 监控象限：色点 + 标题 / 大号百分比主视觉 / 细横条进度 / 副信息。
 * 空载时大数字撑住画面，不会出现空弧/空灰条。
 */
@Composable
private fun MetricCell(
    title: String,
    progress: Float,
    bigText: String,
    subText: String,
    modifier: Modifier = Modifier,
) {
    val tint = tierColor(progress)
    Column(modifier.padding(horizontal = 14.dp, vertical = 16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Box(Modifier.size(8.dp).background(tint, CircleShape))
            Spacer(Modifier.width(6.dp))
            Text(title, fontSize = 12.sp, color = LabelGray, fontWeight = FontWeight.Medium)
        }
        Spacer(Modifier.height(12.dp))
        Text(bigText, fontSize = 30.sp, fontWeight = FontWeight.Bold, color = tint)
        Spacer(Modifier.height(12.dp))
        // 细横条进度
        Box(
            Modifier
                .fillMaxWidth()
                .height(8.dp)
                .clip(RoundedCornerShape(4.dp))
                .background(TrackGray),
        ) {
            Box(
                Modifier
                    .fillMaxWidth(progress.coerceIn(0f, 1f))
                    .height(8.dp)
                    .background(tint),
            )
        }
        Spacer(Modifier.height(8.dp))
        Text(subText, fontSize = 11.sp, color = LabelGray)
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
            verticalArrangement = Arrangement.spacedBy(12.dp),
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

            // ---- 渐变横幅头卡：主机名 + 运行徽章 + IP + 上下行网速 ----
            item {
                Box(
                    Modifier
                        .fillMaxWidth()
                        .clip(RoundedCornerShape(16.dp))
                        .background(Brush.horizontalGradient(listOf(BrandTeal, BrandEmerald)))
                        .padding(16.dp),
                ) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Box(
                            Modifier
                                .size(42.dp)
                                .background(Color.White.copy(alpha = 0.18f), RoundedCornerShape(11.dp)),
                            contentAlignment = Alignment.Center,
                        ) {
                            Icon(Icons.Outlined.Dns, null, tint = Color.White, modifier = Modifier.size(24.dp))
                        }
                        Spacer(Modifier.width(12.dp))
                        Column(Modifier.weight(1f)) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Text(
                                    d.host?.hostname ?: server.name.ifBlank { "服务器" },
                                    color = Color.White, fontSize = 16.sp, fontWeight = FontWeight.Bold,
                                )
                                Spacer(Modifier.width(6.dp))
                                if (uptimeDays > 0) {
                                    Text(
                                        "运行 $uptimeDays 天",
                                        color = Color.White, fontSize = 10.sp, fontWeight = FontWeight.Bold,
                                        modifier = Modifier
                                            .background(Color.White.copy(alpha = 0.22f), RoundedCornerShape(5.dp))
                                            .padding(horizontal = 6.dp, vertical = 2.dp),
                                    )
                                }
                            }
                            Text(
                                server.host.removePrefix("https://").removePrefix("http://"),
                                color = Color.White.copy(alpha = 0.75f), fontSize = 12.sp,
                            )
                        }
                        Column(horizontalAlignment = Alignment.End) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Icon(Icons.Outlined.ArrowDownward, null, tint = Color.White, modifier = Modifier.size(13.dp))
                                Text(" " + humanRate(cur?.recv ?: 0), color = Color.White, fontSize = 12.sp, fontWeight = FontWeight.Bold)
                            }
                            Spacer(Modifier.height(3.dp))
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Icon(Icons.Outlined.ArrowUpward, null, tint = Color.White, modifier = Modifier.size(13.dp))
                                Text(" " + humanRate(cur?.sent ?: 0), color = Color.White, fontSize = 12.sp, fontWeight = FontWeight.Bold)
                            }
                        }
                    }
                }
            }

            // ---- 监控概览区块：一张大卡 2×2 象限 ----
            item {
                SectionTitle("监控概览") { StatePill("实时", BrandTeal) }
            }
            item {
                PanelCard {
                    Column {
                        Row(Modifier.height(IntrinsicSize.Min)) {
                            MetricCell(
                                title = "CPU 使用率",
                                progress = ((cur?.cpu ?: 0.0) / 100.0).toFloat(),
                                bigText = pct(cur?.cpu ?: 0.0),
                                subText = "共 4 核",
                                modifier = Modifier.weight(1f),
                            )
                            Box(Modifier.width(1.dp).fillMaxHeight().background(Hairline))
                            MetricCell(
                                title = "内存使用率",
                                progress = ((cur?.mem_pct ?: 0.0) / 100.0).toFloat(),
                                bigText = pct(cur?.mem_pct ?: 0.0),
                                subText = "${humanSize(cur?.mem_used ?: 0)} / ${humanSize(cur?.mem_total ?: 0)}",
                                modifier = Modifier.weight(1f),
                            )
                        }
                        Box(Modifier.fillMaxWidth().height(1.dp).background(Hairline))
                        Row(Modifier.height(IntrinsicSize.Min)) {
                            MetricCell(
                                title = "系统负载",
                                progress = loadPct,
                                bigText = String.format("%.2f", cur?.load1 ?: 0.0),
                                subText = if (loadPct < 0.5f) "流畅 · 4 核" else if (loadPct < 0.8f) "正常 · 4 核" else "繁忙 · 4 核",
                                modifier = Modifier.weight(1f),
                            )
                            Box(Modifier.width(1.dp).fillMaxHeight().background(Hairline))
                            MetricCell(
                                title = "磁盘 ${disk?.mount ?: "/"}",
                                progress = ((disk?.used_pct ?: 0.0) / 100.0).toFloat(),
                                bigText = pct(disk?.used_pct ?: 0.0),
                                subText = "${humanSize(disk?.used ?: 0)} / ${humanSize(disk?.total ?: 0)}",
                                modifier = Modifier.weight(1f),
                            )
                        }
                    }
                }
            }

            // ---- 运行趋势区块（量程自适应：低占用也有波形感） ----
            item { SectionTitle("运行趋势") }
            item {
                val cpuMax = maxOf(10.0, (d.history.maxOfOrNull { it.cpu } ?: 0.0) * 1.4)
                PanelCard {
                    Text("CPU 使用率", fontSize = 12.sp, color = LabelGray)
                    Spacer(Modifier.height(4.dp))
                    LineChart(d.history.map { it.cpu }, BrandTeal, yMax = cpuMax)
                }
            }
            item {
                val memMax = maxOf(10.0, (d.history.maxOfOrNull { it.mem_pct } ?: 0.0) * 1.4)
                PanelCard {
                    Text("内存使用率", fontSize = 12.sp, color = LabelGray)
                    Spacer(Modifier.height(4.dp))
                    LineChart(d.history.map { it.mem_pct }, AppColors.Blue, yMax = memMax)
                }
            }
            item {
                PanelCard {
                    Text("网络速率", fontSize = 12.sp, color = LabelGray)
                    Text("蓝色=下载 · 橙色=上传", fontSize = 10.sp, color = LabelGray)
                    Spacer(Modifier.height(4.dp))
                    NetChart(d.history.map { it.recv.toDouble() }, d.history.map { it.sent.toDouble() })
                }
            }

            // ---- 系统信息 ----
            item { SectionTitle("系统信息") }
            item {
                PanelCard {
                    d.host?.let {
                        KeyValue("主机名", it.hostname, mono = true)
                        KeyValue("系统", listOfNotNull(it.os, it.platform, it.platform_family).joinToString(" "))
                        val kernel = "${it.kernel_version} ${it.kernel_arch}".trim()
                        if (kernel.isNotEmpty()) KeyValue("内核", kernel, mono = true)
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
