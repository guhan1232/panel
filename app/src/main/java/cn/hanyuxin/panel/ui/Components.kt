// Components.kt —— 通用小组件：卡片、指标卡、Canvas 折线图、空态/错误条、设置行/分段器/搜索框等重构组件。
package cn.hanyuxin.panel.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.KeyboardArrowRight
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/** 通用圆角卡片。 */
@Composable
fun PanelCard(modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    Card(
        modifier = modifier.fillMaxWidth(),
        shape = RoundedCornerShape(14.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
        elevation = CardDefaults.cardElevation(defaultElevation = 0.dp),
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outline.copy(alpha = 0.45f)),
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

// ===== 页面重构共享组件（设置行 / 分段器 / 搜索框 / 徽章 / 标签）=====

/** 品牌渐变（头部英雄卡背景）：青绿 → 天蓝。 */
val PanelBrandBrush = Brush.linearGradient(listOf(Color(0xFF0FA79A), Color(0xFF0E7FC0)))

/** 图标徽章：列表行左侧的圆角着色图标容器。 */
@Composable
fun IconBadge(
    icon: ImageVector,
    tint: Color,
    modifier: Modifier = Modifier,
    container: Color = tint.copy(alpha = 0.14f),
    badge: Dp = 38.dp,
    iconSize: Dp = 20.dp,
) {
    Box(
        modifier = modifier.size(badge).background(container, RoundedCornerShape(11.dp)),
        contentAlignment = Alignment.Center,
    ) {
        Icon(icon, contentDescription = null, tint = tint, modifier = Modifier.size(iconSize))
    }
}

/** 设置分组标签（iOS 设置风格的小节标题）。 */
@Composable
fun SectionLabel(text: String) {
    Text(
        text,
        style = MaterialTheme.typography.labelMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(start = 6.dp, bottom = 2.dp),
    )
}

/** 设置分组卡：行自带水平内边距，行间用 GroupDivider 分隔。 */
@Composable
fun GroupCard(content: @Composable ColumnScope.() -> Unit) {
    Card(
        shape = RoundedCornerShape(16.dp),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
    ) {
        Column(Modifier.padding(vertical = 4.dp), content = content)
    }
}

/** 分组内行间细分割线（左缩进对齐文字区）。 */
@Composable
fun GroupDivider() {
    HorizontalDivider(
        Modifier.padding(start = 64.dp),
        thickness = 0.7.dp,
        color = MaterialTheme.colorScheme.outline.copy(alpha = 0.25f),
    )
}

/** 设置行：图标徽章 + 标题/副标题 + 可选尾部组件 + 可选箭头，整行可点。 */
@Composable
fun SettingsRow(
    icon: ImageVector,
    iconTint: Color,
    title: String,
    sub: String,
    danger: Boolean = false,
    chevron: Boolean = true,
    trailing: (@Composable () -> Unit)? = null,
    onClick: (() -> Unit)? = null,
) {
    val tint = if (danger) MaterialTheme.colorScheme.error else iconTint
    Row(
        Modifier
            .fillMaxWidth()
            .then(if (onClick != null) Modifier.clickable { onClick() } else Modifier)
            .padding(horizontal = 12.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        IconBadge(icon, tint)
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f)) {
            Text(
                title,
                fontSize = 14.sp,
                fontWeight = FontWeight.Medium,
                color = if (danger) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface,
            )
            if (sub.isNotBlank()) {
                Text(sub, fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 2)
            }
        }
        if (trailing != null) {
            Spacer(Modifier.width(8.dp))
            trailing()
        }
        if (chevron) {
            Icon(
                Icons.AutoMirrored.Outlined.KeyboardArrowRight, null,
                tint = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.45f),
                modifier = Modifier.size(18.dp),
            )
        }
    }
}

/** 分段选择器：胶囊式二/三选项切换（替代 TabRow / FilterChip）。 */
@Composable
fun Segmented(options: List<String>, selected: Int, onSelect: (Int) -> Unit, modifier: Modifier = Modifier) {
    Box(
        modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.55f), RoundedCornerShape(12.dp))
            .padding(4.dp),
    ) {
        Row {
            options.forEachIndexed { i, label ->
                val sel = i == selected
                Box(
                    Modifier
                        .weight(1f)
                        .clip(RoundedCornerShape(9.dp))
                        .background(if (sel) MaterialTheme.colorScheme.primary else Color.Transparent)
                        .clickable { onSelect(i) }
                        .padding(vertical = 8.dp),
                    contentAlignment = Alignment.Center,
                ) {
                    Text(
                        label,
                        fontSize = 13.sp,
                        fontWeight = if (sel) FontWeight.SemiBold else FontWeight.Normal,
                        color = if (sel) MaterialTheme.colorScheme.onPrimary else MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

/** 圆角搜索框（本地过滤）。 */
@Composable
fun SearchField(value: String, onValueChange: (String) -> Unit, hint: String, modifier: Modifier = Modifier) {
    Row(
        modifier
            .fillMaxWidth()
            .background(MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.55f), RoundedCornerShape(12.dp))
            .padding(horizontal = 12.dp, vertical = 10.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(Icons.Outlined.Search, null, tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(17.dp))
        Spacer(Modifier.width(8.dp))
        Box(Modifier.weight(1f)) {
            if (value.isEmpty()) {
                Text(hint, fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            BasicTextField(
                value = value,
                onValueChange = onValueChange,
                singleLine = true,
                textStyle = TextStyle(fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurface),
                modifier = Modifier.fillMaxWidth(),
            )
        }
        if (value.isNotEmpty()) {
            Icon(
                Icons.Outlined.Close, "清空",
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.size(15.dp).clickable { onValueChange("") },
            )
        }
    }
}

/** 状态圆点。 */
@Composable
fun StatusDot(color: Color, size: Dp = 8.dp) {
    Box(Modifier.size(size).background(color, CircleShape))
}

/** 小标签胶囊（元信息/状态）。 */
@Composable
fun TagChip(text: String, tint: Color, modifier: Modifier = Modifier) {
    Text(
        text,
        fontSize = 10.sp,
        fontWeight = FontWeight.Medium,
        color = tint,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
        modifier = modifier
            .background(tint.copy(alpha = 0.14f), RoundedCornerShape(6.dp))
            .padding(horizontal = 6.dp, vertical = 2.dp),
    )
}

/** 深色代码块卡片（日志/命令输出）。 */
@Composable
fun CodeBlock(text: String, modifier: Modifier = Modifier, height: Dp = 140.dp) {
    Box(
        modifier
            .fillMaxWidth()
            .height(height)
            .background(Color(0xFF0A1020), RoundedCornerShape(10.dp))
            .padding(10.dp),
    ) {
        Text(text, fontSize = 11.sp, fontFamily = FontFamily.Monospace, color = Color(0xFFA5D6C8))
    }
}
