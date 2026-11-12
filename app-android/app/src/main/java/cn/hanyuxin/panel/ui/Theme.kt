// Theme.kt —— 深色 Material3 主题（服务器管理场景固定深色）。
package cn.hanyuxin.panel.ui

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color

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

@Composable
fun AppTheme(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = DarkScheme, content = content)
}
