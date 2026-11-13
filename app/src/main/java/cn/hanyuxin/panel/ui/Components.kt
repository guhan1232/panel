// Components.kt —— 通用小组件：卡片、指标卡、Canvas 折线图、空态/错误条。
package cn.hanyuxin.panel.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/** 通用圆角卡片。 */
@Composable
fun PanelCard(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        content = { Column(Modifier.padding(14.dp)) { content() } },
    )
}

/** 指标卡：图标 + 标签 + 大数值 + 副信息。 */
@Composable
fun StatCard(
    icon: ImageVector,
    label: String,
    value: String,
    sub: String? = null,
    tint: Color = MaterialTheme.colorScheme.primary,
    modifier: Modifier = Modifier,
) {
    PanelCard(modifier) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(icon, contentDescription = label, tint = tint, modifier = Modifier.size(22.dp))
            Spacer(Modifier.width(8.dp))
            Text(label, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Spacer(Modifier.height(6.dp))
        Text(value, fontSize = 24.sp, fontWeight = FontWeight.Bold, color = tint)
        if (sub != null) {
            Text(sub, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

/** 简易折线图（0..100 百分比序列），免第三方依赖。 */
@Composable
fun LineChart(values: List<Double>, color: Color, modifier: Modifier = Modifier, yMax: Double = 100.0) {
    Canvas(modifier = modifier.fillMaxWidth().height(110.dp)) {
        if (values.size < 2) return@Canvas
        val stepX = size.width / (values.size - 1)
        val path = Path()
        values.forEachIndexed { i, v ->
            val y = size.height - (v.coerceIn(0.0, yMax) / yMax * size.height).toFloat()
            if (i == 0) path.moveTo(0f, y) else path.lineTo(i * stepX, y)
        }
        drawPath(path, color, style = Stroke(width = 4f))
        // 填充淡色区域
        val fill = Path().apply {
            addPath(path)
            lineTo(size.width, size.height)
            lineTo(0f, size.height)
            close()
        }
        drawPath(fill, color.copy(alpha = 0.15f))
    }
}

/** 网速折线图（自动按峰值归一）。 */
@Composable
fun NetChart(recv: List<Double>, sent: List<Double>, modifier: Modifier = Modifier) {
    val peak = (recv + sent).maxOrNull()?.takeIf { it > 0 } ?: 1.0
    Column {
        LineChart(recv, Color(0xFF38BDF8), modifier, yMax = peak)
        LineChart(sent, Color(0xFFF59E0B), modifier, yMax = peak)
    }
}

@Composable
fun EmptyView(text: String) {
    Column(
        Modifier.fillMaxWidth().padding(40.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(text, color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 14.sp)
    }
}

@Composable
fun ErrorBanner(text: String?) {
    if (text.isNullOrBlank()) return
    Text(
        text,
        color = MaterialTheme.colorScheme.error,
        fontSize = 13.sp,
        modifier = Modifier.fillMaxWidth().background(MaterialTheme.colorScheme.error.copy(alpha = 0.12f), RoundedCornerShape(8.dp)).padding(10.dp),
    )
}

@Composable
fun LoadingView() {
    Column(
        Modifier.fillMaxWidth().padding(40.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        CircularProgressIndicator()
    }
}

@Composable
fun KeyValue(k: String, v: String, mono: Boolean = false) {
    Row(Modifier.fillMaxWidth().padding(vertical = 3.dp), horizontalArrangement = Arrangement.SpaceBetween) {
        Text(k, fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text(v, fontSize = 13.sp, fontFamily = if (mono) FontFamily.Monospace else null, fontWeight = FontWeight.Medium)
    }
}

/** 字节人性化显示。 */
fun humanSize(bytes: Long): String {
    if (bytes < 0) return "-"
    val units = listOf("B", "KB", "MB", "GB", "TB")
    var v = bytes.toDouble()
    var i = 0
    while (v >= 1024 && i < units.size - 1) { v /= 1024; i++ }
    return if (i == 0) "${bytes}B" else String.format("%.1f%s", v, units[i])
}

/** 网速人性化显示（B/s）。 */
fun humanRate(bps: Long): String = humanSize(bps) + "/s"

/** 百分比显示。 */
fun pct(v: Double): String = String.format("%.1f%%", v)
