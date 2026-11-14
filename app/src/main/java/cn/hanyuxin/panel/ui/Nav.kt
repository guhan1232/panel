// Nav.kt —— 轻量路由栈（Overlay 覆盖式导航）+ 会话状态 + 根装配。
package cn.hanyuxin.panel.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.Language
import androidx.compose.material.icons.outlined.Person
import androidx.compose.material.icons.outlined.Speed
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Icon
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import cn.hanyuxin.panel.data.Http
import cn.hanyuxin.panel.data.QrData
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.ServerStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/** 全屏覆盖层路由（压栈式）。 */
object Router {
    sealed class Route {
        data class AddServer(val onDone: () -> Unit) : Route()
        data class Scanner(val onQr: (QrData) -> Unit, val onClose: () -> Unit) : Route()
        data class Login(val server: ServerInfo, val qrKey: String, val onLoggedIn: (ServerInfo) -> Unit, val onClose: () -> Unit) : Route()
        data class BindLogin(val server: ServerInfo, val bindKey: String, val onClose: () -> Unit) : Route()
        data class QrApprove(val server: ServerInfo, val key: String, val onDone: () -> Unit) : Route()
        data class Editor(val server: ServerInfo, val path: String, val onClose: () -> Unit) : Route()
        data class Alerts(val server: ServerInfo, val onClose: () -> Unit) : Route()
        // 全功能页（服务/运维宫格入口）
        data class Sites(val server: ServerInfo) : Route()
        data class Databases(val server: ServerInfo) : Route()
        data class AppStore(val server: ServerInfo) : Route()
        data class Docker(val server: ServerInfo) : Route()
        data class Terminal(val server: ServerInfo) : Route()
        data class Cron(val server: ServerInfo) : Route()
        data class Proc(val server: ServerInfo) : Route()
        data class Nodes(val server: ServerInfo) : Route()
        data class Firewall(val server: ServerInfo) : Route()
        data class Certs(val server: ServerInfo) : Route()
        data class Audit(val server: ServerInfo) : Route()
        data class Notify(val server: ServerInfo) : Route()
        data class Users(val server: ServerInfo) : Route()
        data class Settings(val server: ServerInfo) : Route()
    }

    private val _stack = MutableStateFlow<List<Route>>(emptyList())
    val stack: StateFlow<List<Route>> = _stack
    fun push(r: Route) { _stack.value = _stack.value + r }
    fun pop() { _stack.value = _stack.value.dropLast(1) }
}

/** 当前登录会话（选中的服务器）。 */
object Session {
    val current = MutableStateFlow<ServerInfo?>(null)
    fun enter(s: ServerInfo) { android.util.Log.d("AZAPP", "enter "+s.host); current.value = s; ServerStore.setLastHost(s.host) }
    /** 退出当前面板：clearToken=false（切换服务器）保留令牌，切回免密直进；
     *  clearToken=true（401 失效/退出登录）清除令牌。 */
    fun leave(clearToken: Boolean = false) {
        if (clearToken) {
            current.value?.let {
                ServerStore.setToken(it.host, null)
                Http.invalidate(it.host)
            }
        }
        current.value = null
        ServerStore.setLastHost(null)
    }
}

/** 统一 API 守卫：401 → 退出会话回登录页；其余错误回调展示。返回 null 表示失败。 */
suspend fun <T> apiGuard(onError: (Exception) -> Unit = {}, block: suspend () -> T): T? =
    try {
        block()
    } catch (e: cn.hanyuxin.panel.data.UnauthorizedException) {
        Session.leave(clearToken = true)
        null
    } catch (e: Exception) {
        onError(e)
        null
    }

/** 扫码分发：已有该服务器的有效会话 → 直接授权；否则先落库再走登录（登录后自动继续授权）。 */
fun handleScan(qr: QrData) {
    // 绑定码（Web 生成的绑定密钥）：保存服务器+持久化密钥 → 免密 bind-login → 自动进入主界面。
    // 密钥落库（bindKey）：此后令牌失效（过期/面板侧吊销）点卡片即免密重连，无需账号密码。
    if (qr.bindKey.isNotBlank()) {
        ServerStore.upsert(qr.host, qr.entrance, bindKey = qr.bindKey)
        val s = ServerStore.byHost(qr.host) ?: return
        Router.pop() // 关闭扫码页
        Router.push(Router.Route.BindLogin(s, qr.bindKey, onClose = { Router.pop() }))
        return
    }
    val existing = ServerStore.byHost(qr.host)
    if (existing != null && ServerStore.token(qr.host) != null) {
        Router.push(Router.Route.QrApprove(existing, qr.key, onDone = { Router.pop(); Router.pop() }))
    } else {
        ServerStore.upsert(qr.host, qr.entrance)
        val s = ServerStore.byHost(qr.host) ?: return
        Router.pop() // 关闭扫码页
        Router.push(Router.Route.Login(s, qr.key, onLoggedIn = { Session.enter(it) }, onClose = { Router.pop() }))
    }
}

@Composable
fun RootApp() {
    val overlay = Router.stack.collectAsState().value.lastOrNull()
    val current = Session.current.collectAsState().value
    val servers by ServerStore.servers.collectAsState()

    LaunchedEffect(Unit) {
        val last = ServerStore.lastHost()
        if (Session.current.value == null && last != null) {
            val s = ServerStore.byHost(last)
            if (s != null && ServerStore.token(last) != null) {
                Session.enter(s) // 有效令牌：直接进入
            } else if (s != null && s.bindKey.isNotBlank()) {
                // 令牌失效但有绑定密钥：启动即免密 bind-login 重连，无感直进
                Router.push(Router.Route.BindLogin(s, s.bindKey, onClose = { Router.pop() }))
            }
        }
    }
    BackHandler(enabled = overlay != null) { Router.pop() }

    when (val o = overlay) {
        is Router.Route.AddServer -> AddServerScreen { Router.pop() }
        is Router.Route.Scanner -> ScannerScreen(o.onQr, o.onClose)
        is Router.Route.Login -> LoginScreen(o.server, o.qrKey, o.onLoggedIn, o.onClose)
        is Router.Route.BindLogin -> BindLoginScreen(o.server, o.bindKey, o.onClose)
        is Router.Route.QrApprove -> QrApproveScreen(o.server, o.key, o.onDone)
        is Router.Route.Editor -> EditorScreen(o.server, o.path, o.onClose)
        is Router.Route.Alerts -> AlertsScreen(o.server, o.onClose)
        is Router.Route.Sites -> SitesScreen(o.server)
        is Router.Route.Databases -> DatabaseScreen(o.server)
        is Router.Route.AppStore -> AppStoreScreen(o.server)
        is Router.Route.Docker -> DockerScreen(o.server)
        is Router.Route.Terminal -> TerminalScreen(o.server)
        is Router.Route.Cron -> CronScreen(o.server)
        is Router.Route.Proc -> ProcScreen(o.server)
        is Router.Route.Nodes -> NodesScreen(o.server)
        is Router.Route.Firewall -> FirewallScreen(o.server)
        is Router.Route.Certs -> CertsScreen(o.server)
        is Router.Route.Audit -> AuditScreen(o.server)
        is Router.Route.Notify -> NotifyScreen(o.server)
        is Router.Route.Users -> UsersScreen(o.server)
        is Router.Route.Settings -> SettingsScreen(o.server)
        null -> when {
            current != null -> MainScaffold()
            else -> ServersScreen(
                onEnter = { s ->
                    android.util.Log.d("AZAPP", "card click token="+(ServerStore.token(s.host)!=null)+" host="+s.host)
                    if (ServerStore.token(s.host) != null) {
                        Session.enter(s)
                    } else if (s.bindKey.isNotBlank()) {
                        // 令牌失效但有绑定密钥：免密 bind-login 直接进（不要求账号密码）
                        Router.push(Router.Route.BindLogin(s, s.bindKey, onClose = { Router.pop() }))
                    } else {
                        // 无令牌无密钥的服务器先进登录页（预填地址与入口码），登录成功后再进入主界面
                        Router.push(Router.Route.Login(s, "", onLoggedIn = { Session.enter(it) }, onClose = { Router.pop() }))
                    }
                },
                onAdd = { Router.push(Router.Route.AddServer {}) },
                onScan = { Router.push(Router.Route.Scanner(onQr = { handleScan(it) }, onClose = { Router.pop() })) },
            )
        }
    }
}

@Composable
fun MainScaffold() {
    val server = Session.current.collectAsState().value ?: return
    var tab by rememberSaveable { mutableIntStateOf(0) }
    // 底部五 Tab 对应 Web 端侧边栏分组：概览 / 站点与服务 / 系统运维 / 文件 / 我的
    val tabs = listOf(
        "概览" to Icons.Outlined.Speed,
        "服务" to Icons.Outlined.Language,
        "运维" to Icons.Outlined.Settings,
        "文件" to Icons.Outlined.Folder,
        "我的" to Icons.Outlined.Person,
    )
    Scaffold(
        bottomBar = {
            NavigationBar {
                tabs.forEachIndexed { i, (label, icon) ->
                    NavigationBarItem(selected = tab == i, onClick = { tab = i }, icon = { Icon(icon, contentDescription = label) }, label = { Text(label) })
                }
            }
        },
    ) { pad ->
        Column(Modifier.padding(pad).fillMaxSize()) {
            when (tab) {
                0 -> DashboardScreen(server)
                1 -> ServiceHubScreen(server)
                2 -> OpsHubScreen(server)
                3 -> FilesScreen(server)
                else -> MeScreen(server)
            }
        }
    }
}
