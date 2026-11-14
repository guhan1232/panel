// DashboardScreen.kt —— 概览（宝塔式）：渐变横幅头卡（主机名/运行徽章/IP/上下行网速）
// + 监控大卡 2×2 象限（负载温度计/CPU·内存圆弧/磁盘横条，色阶绿→黄→红）+ 趋势图 + 系统信息。
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
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

private val BrandTeal = Color(0xFF0D9488)
private val BrandEmerald = Color(0xFF10B981)
// OkGreen 复用 MeScreens.kt 同包定义（0xFF22C55E）
private val WarnAmber = Color(0xFFF59E0B)
private val DangerRed = Color(0xFFEF4444)
private val TrackGray = Color(0xFFEDF0F3)
private val Hairline = Color(0xFFEEF0F3)

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

/** 绿色小徽章（运行天数 / 已用内存 / 总容量）。 */
@Composable
private fun MiniBadge(text: String, bg: Color = OkGreen) {
    Text(
        text,
        fontSize = 11.sp, fontWeight = FontWeight.Bold,
        color = Color.White,
        modifier = Modifier
            .background(bg, RoundedCornerShape(5.dp))
            .padding(horizontal = 6.dp, vertical = 2.dp),
    )
}

/** 浅底状态 pill（流畅/正常/繁忙）。 */
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

/** 圆弧仪表：底环 + 进度弧（-90° 起、圆头）+ 中央百分比，随进度着色。 */
@Composable
private fun GaugeArc(progress: Float, modifier: Modifier = Modifier) {
    val p = progress.coerceIn(0f, 1f)
    val tint = tierColor(p)
    Box(modifier.size(92.dp), contentAlignment = Alignment.Center) {
        Canvas(Modifier.size(92.dp)) {
            val stroke = 10.dp.toPx()
            drawArc(
                TrackGray, startAngle = 0f, sweepAngle = 360f, useCenter = false,
                style = Stroke(stroke, cap = StrokeCap.Round),
            )
            drawArc(
                tint, startAngle = -90f, sweepAngle = 360f * p, useCenter = false,
                style = Stroke(stroke, cap = StrokeCap.Round),
            )
        }
        Text("${(p * 100).toInt()}%", fontSize = 18.sp, fontWeight = FontWeight.Bold, color = tint)
    }
}

/** 负载温度计：百分比大字在上 + 细竖条（底部填充、色阶）。 */
@Composable
private fun LoadBar(progress: Float, modifier: Modifier = Modifier) {
    val p = progress.coerceIn(0f, 1f)
    val tint = tierColor(p)
    Column(modifier, horizontalAlignment = Alignment.CenterHorizontally) {
        Text("${(p * 100).toInt()}%", fontSize = 18.sp, fontWeight = FontWeight.Bold, color = tint)
        Spacer(Modifier.height(8.dp))
        Box(
            Modifier
                .width(26.dp)
                .height(104.dp)
                .clip(RoundedCornerShape(13.dp))
                .background(TrackGray),
        ) {
            Box(
                Modifier
                    .align(Alignment.BottomCenter)
                    .fillMaxWidth()
                    .height(maxOf(10.dp, (104 * p).dp))
                    .background(tint),
            )
        }
    }
}

/** 水平进度条（磁盘）。 */
@Composable
private fun HBar(progress: Float, tint: Color, modifier: Modifier = Modifier) {
    Box(
        modifier
            .fillMaxWidth()
            .height(12.dp)
            .clip(RoundedCornerShape(6.dp))
            .background(TrackGray),
    ) {
        Box(
            Modifier
                .fillMaxWidth(progress.coerceIn(0f, 1f))
                .height(12.dp)
                .background(tint),
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

            // ---- 监控概览区块 ----
            item {
                SectionTitle("监控概览") {
                    StatePill("实时", BrandTeal)
                }
            }

            // 监控大卡：2×2 象限 + 细分隔线
            item {
                PanelCard {
                    Row {
                        // 象限 1：负载
                        Column(
                            Modifier.weight(1f).height(180.dp),
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.Center,
                        ) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Text("负载", fontSize = 13.sp, fontWeight = FontWeight.SemiBold, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                Spacer(Modifier.width(6.dp))
                                StatePill(
                                    if (loadPct < 0.5f) "流畅" else if (loadPct < 0.8f) "正常" else "繁忙",
                                    tierColor(loadPct),
                                )
                            }
                            Spacer(Modifier.height(10.dp))
                            LoadBar(loadPct)
                            Spacer(Modifier.height(10.dp))
                            Text("负载 ${String.format("%.2f", cur?.load1 ?: 0.0)}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                        // 竖分隔线
                        Box(Modifier.width(1.dp).height(170.dp).background(Hairline))
                        // 象限 2：CPU
                        Column(
                            Modifier.weight(1f).height(180.dp),
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.Center,
                        ) {
                            Text("CPU 使用率", fontSize = 13.sp, fontWeight = FontWeight.SemiBold, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            Spacer(Modifier.height(12.dp))
                            GaugeArc(((cur?.cpu ?: 0.0) / 100.0).toFloat())
                        }
                    }
                    // 横分隔线
                    Box(Modifier.fillMaxWidth().height(1.dp).background(Hairline))
                    Row {
                        // 象限 3：内存
                        Column(
                            Modifier.weight(1f).height(180.dp),
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.Center,
                        ) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Text("内存使用率", fontSize = 13.sp, fontWeight = FontWeight.SemiBold, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                Spacer(Modifier.width(6.dp))
                                MiniBadge(humanSize(cur?.mem_used ?: 0))
                            }
                            Spacer(Modifier.height(12.dp))
                            GaugeArc(((cur?.mem_pct ?: 0.0) / 100.0).toFloat())
                        }
                        // 竖分隔线
                        Box(Modifier.width(1.dp).height(170.dp).background(Hairline))
                        // 象限 4：磁盘
                        Column(
                            Modifier.weight(1f).height(180.dp).padding(horizontal = 12.dp),
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.Center,
                        ) {
                            Row(verticalAlignment = Alignment.CenterVertically) {
                                Text(disk?.mount ?: "/", fontSize = 13.sp, fontWeight = FontWeight.SemiBold, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                Spacer(Modifier.width(6.dp))
                                MiniBadge(humanSize(disk?.total ?: 0))
                            }
                            Spacer(Modifier.height(14.dp))
                            val dpct = ((disk?.used_pct ?: 0.0) / 100.0).toFloat()
                            HBar(dpct, tierColor(dpct))
                            Spacer(Modifier.height(12.dp))
                            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.Bottom) {
                                Text("${(dpct * 100).toInt()}%", fontSize = 18.sp, fontWeight = FontWeight.Bold, color = tierColor(dpct))
                                Spacer(Modifier.weight(1f))
                                Text("${humanSize(disk?.used ?: 0)} / ${humanSize(disk?.total ?: 0)}", fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                            }
                        }
                    }
                }
            }

            // ---- 运行趋势区块 ----
            item { SectionTitle("运行趋势") }
            item {
                PanelCard {
                    Text("CPU 使用率", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Spacer(Modifier.height(4.dp))
                    LineChart(d.history.map { it.cpu }, BrandTeal)
                }
            }
            item {
                PanelCard {
                    Text("内存使用率", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Spacer(Modifier.height(4.dp))
                    LineChart(d.history.map { it.mem_pct }, AppColors.Blue)
                }
            }
            item {
                PanelCard {
                    Text("网络速率", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Text("蓝色=下载 · 橙色=上传", fontSize = 10.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
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
