// SitesScreen.kt —— 网站管理：列表 + 启停开关 + 详情（完整创建/编辑引导用 Web 端）。
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
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.Api
import cn.hanyuxin.panel.data.ApiException
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.Site
import cn.hanyuxin.panel.data.clientFor
import kotlinx.coroutines.launch

@Composable
fun SitesScreen(server: ServerInfo) {
    val scope = rememberCoroutineScope()
    var sites by remember(server.host) { mutableStateOf<List<Site>?>(null) }
    var err by remember { mutableStateOf<String?>(null) }
    var toggling by remember { mutableStateOf<Long?>(null) }
    var detail by remember { mutableStateOf<Site?>(null) }
    var reload by remember { mutableStateOf(0) }

    LaunchedEffect(server.host, reload) {
        apiGuard(onError = { err = it.message ?: "加载失败" }) { Api.sites(clientFor(server)) }?.let { sites = it; err = null }
    }

    Column(Modifier.fillMaxSize().padding(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("网站", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            IconButton(onClick = { reload++ }) { Icon(Icons.Outlined.Refresh, "刷新") }
        }
        ErrorBanner(err)
        val list = sites
        when {
            list == null -> LoadingView()
            list.isEmpty() -> EmptyView("还没有网站，创建站点请用 Web 端")
            else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                items(list.filterNot { it.system }, key = { it.id }) { s ->
                    Card(
                        shape = RoundedCornerShape(14.dp),
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                        modifier = Modifier.fillMaxWidth().clickable { detail = s },
                    ) {
                        Row(Modifier.padding(14.dp), verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text(s.name, fontWeight = FontWeight.SemiBold)
                                Text(
                                    (s.domains.ifBlank { s.upstream.ifBlank { s.root } }).take(60),
                                    fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                            if (toggling == s.id) {
                                CircularProgressIndicator(Modifier.height(20.dp).width(20.dp), strokeWidth = 2.dp)
                            } else {
                                Switch(
                                    checked = s.enabled,
                                    onCheckedChange = {
                                        toggling = s.id
                                        scope.launch {
                                            apiGuard(onError = { err = it.message }) { Api.toggleSite(clientFor(server), s.id) }?.let {
                                                reload++
                                            }
                                            toggling = null
                                        }
                                    },
                                )
                            }
                        }
                    }
                }
            }
        }
    }

    detail?.let { s ->
        AlertDialog(
            onDismissRequest = { detail = null },
            title = { Text(s.name) },
            text = {
                Column {
                    KeyValue("类型", listOfNotNull(s.type, s.sub_type.takeIf { it.isNotBlank() }).joinToString("/"))
                    if (s.domains.isNotBlank()) KeyValue("域名", s.domains, mono = true)
                    if (s.root.isNotBlank()) KeyValue("根目录", s.root, mono = true)
                    if (s.upstream.isNotBlank()) KeyValue("反代目标", s.upstream, mono = true)
                    KeyValue("状态", if (s.enabled) "运行中" else "已停止")
                    Spacer(Modifier.height(8.dp))
                    Text("创建与完整编辑请在 Web 端操作", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            },
            confirmButton = { TextButton(onClick = { detail = null }) { Text("关闭") } },
        )
    }
}
