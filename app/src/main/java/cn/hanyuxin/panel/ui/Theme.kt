// Theme.kt —— 对齐 Web 设计体系：浅蓝灰底 #F4F6FA + 白卡 + 青绿主色 #0D9488；深色跟随系统（保留服务器管理深色）。
package cn.hanyuxin.panel.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

private val LightScheme = lightColorScheme(
    primary = Color(0xFF0D9488),
    onPrimary = Color(0xFFFFFFFF),
    primaryContainer = Color(0xFFCCFBF1),
    onPrimaryContainer = Color(0xFF064E46),
    secondary = Color(0xFF0E7FC0),
    onSecondary = Color(0xFFFFFFFF),
    secondaryContainer = Color(0xFFE0F2FE),
    onSecondaryContainer = Color(0xFF0C4A6E),
    background = Color(0xFFF4F6FA),
    onBackground = Color(0xFF1F2937),
    surface = Color(0xFFFFFFFF),
    onSurface = Color(0xFF1F2937),
    surfaceVariant = Color(0xFFEEF2F7),
    onSurfaceVariant = Color(0xFF64748B),
    error = Color(0xFFDC2626),
    onError = Color(0xFFFFFFFF),
    outline = Color(0xFFD8DFE8),
)

private val DarkScheme = darkColorScheme(
    primary = Color(0xFF2DD4BF),
    onPrimary = Color(0xFF04211D),
    primaryContainer = Color(0xFF0F3D38),
    onPrimaryContainer = Color(0xFFB2F5EC),
    secondary = Color(0xFF60A5FA),
    onSecondary = Color(0xFF0B2447),
    background = Color(0xFF0B1220),
    onBackground = Color(0xFFE2E8F0),
    surface = Color(0xFF121B2C),
    onSurface = Color(0xFFE2E8F0),
    surfaceVariant = Color(0xFF1C2740),
    onSurfaceVariant = Color(0xFF94A3B8),
    error = Color(0xFFF87171),
    onError = Color(0xFF2B0B0B),
    outline = Color(0xFF2C3A55),
)

/** 模块彩色板（亮底可读，对齐 Web --wm-p-*）：状态/图表/宫格图标着色用。 */
object AppColors {
    val Teal = Color(0xFF0D9488)
    val Blue = Color(0xFF0E7FC0)
    val Sky = Color(0xFF0284C7)
    val Violet = Color(0xFF7C3AED)
    val Amber = Color(0xFFD97706)
    val Emerald = Color(0xFF059669)
    val Rose = Color(0xFFE11D48)
    val Indigo = Color(0xFF4F46E5)
    val Slate = Color(0xFF64748B)
    val Green = Color(0xFF16A34A)
}

@Composable
fun AppTheme(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = if (isSystemInDarkTheme()) DarkScheme else LightScheme, content = content)
}
