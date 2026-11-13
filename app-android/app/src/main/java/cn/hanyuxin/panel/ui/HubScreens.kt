// HubScreens.kt —— 「服务」「运维」宫格入口页：对应 Web 端侧边栏分组，点卡片进入各功能页。
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
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.AccountTree
import androidx.compose.material.icons.outlined.Alarm
import androidx.compose.material.icons.outlined.Cloud
import androidx.compose.material.icons.outlined.Extension
import androidx.compose.material.icons.outlined.Language
import androidx.compose.material.icons.outlined.Memory
import androidx.compose.material.icons.outlined.Schedule
import androidx.compose.material.icons.outlined.Security
import androidx.compose.material.icons.outlined.Storage
import androidx.compose.material.icons.outlined.Terminal
import androidx.compose.material.icons.outlined.VpnKey
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.ServerInfo

private data class HubItem(val title: String, val desc: String, val icon: ImageVector, val route: Router.Route)

/** 站点与服务宫格：网站 / 数据库 / 软件商店 / Docker / 终端。 */
@Composable
fun ServiceHubScreen(server: ServerInfo) {
    val items = listOf(
        HubItem("网站", "站点与反向代理", Icons.Outlined.Language, Router.Route.Sites(server)),
        HubItem("数据库", "MySQL / PostgreSQL", Icons.Outlined.Storage, Router.Route.Databases(server)),
        HubItem("软件商店", "应用安装与管理", Icons.Outlined.Extension, Router.Route.AppStore(server)),
        HubItem("Docker", "容器 / 镜像管理", Icons.Outlined.Cloud, Router.Route.Docker(server)),
        HubItem("终端", "服务器命令行", Icons.Outlined.Terminal, Router.Route.Terminal(server)),
    )
    HubGrid("站点与服务", items)
}

/** 系统运维宫格：计划任务 / 告警 / 进程 / 节点 / 防火墙 / SSL 证书。 */
@Composable
fun OpsHubScreen(server: ServerInfo) {
    val items = listOf(
        HubItem("计划任务", "定时脚本与备份", Icons.Outlined.Schedule, Router.Route.Cron(server)),
        HubItem("告警策略", "指标告警与事件", Icons.Outlined.Alarm, Router.Route.Alerts(server, onClose = { Router.pop() })),
        HubItem("进程管理", "进程与服务控制", Icons.Outlined.Memory, Router.Route.Proc(server)),
        HubItem("节点管理", "多机监控与执行", Icons.Outlined.AccountTree, Router.Route.Nodes(server)),
        HubItem("防火墙", "端口规则与防扫描", Icons.Outlined.Security, Router.Route.Firewall(server)),
        HubItem("SSL 证书", "证书申请与续期", Icons.Outlined.VpnKey, Router.Route.Certs(server)),
    )
    HubGrid("系统运维", items)
}

@Composable
private fun HubGrid(title: String, items: List<HubItem>) {    Column(Modifier.fillMaxSize().padding(16.dp)) {
        Text(title, style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)
        Spacer(Modifier.height(12.dp))
        LazyVerticalGrid(
            columns = GridCells.Fixed(2),
            verticalArrangement = Arrangement.spacedBy(10.dp),
            horizontalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            items(items) { it ->
                Card(
                    shape = RoundedCornerShape(14.dp),
                    colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                    modifier = Modifier.fillMaxWidth().height(92.dp).clickable { Router.push(it.route) },
                ) {
                    Column(Modifier.padding(14.dp)) {
                        Row(verticalAlignment = Alignment.CenterVertically) {
                            Icon(it.icon, contentDescription = it.title, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(22.dp))
                            Spacer(Modifier.width(8.dp))
                            Text(it.title, fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
                        }
                        Spacer(Modifier.height(6.dp))
                        Text(it.desc, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                }
            }
        }
    }
}
