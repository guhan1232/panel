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
import androidx.compose.material.icons.outlined.Storefront
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
    fun leave() {
        // 会话失效（401）时同步清除本地失效令牌，否则列表仍显示「已登录」，
        // 用户点击进入后再次 401，形成「登录不进去」死循环
        current.value?.let { ServerStore.setToken(it.host, null); Http.invalidate(it.host) }
        current.value = null
        ServerStore.setLastHost(null)
    }
}

/** 统一 API 守卫：401 → 退出会话回登录页；其余错误回调展示。返回 null 表示失败。 */
suspend fun <T> apiGuard(onError: (Exception) -> Unit = {}, block: suspend () -> T): T? =
    try {
        block()
    } catch (e: cn.hanyuxin.panel.data.UnauthorizedException) {
        Session.leave()
        null
    } catch (e: Exception) {
        onError(e)
        null
    }

/** 扫码分发：已有该服务器的有效会话 → 直接授权；否则先落库再走登录（登录后自动继续授权）。 */
fun handleScan(qr: QrData) {
    // 绑定码（Web 生成的绑定密钥）：保存服务器 → 免密 bind-login → 自动进入主界面
    if (qr.bindKey.isNotBlank()) {
        ServerStore.upsert(qr.host, qr.entrance)
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
        if (Session.current.value == null && last != null && ServerStore.token(last) != null) {
            ServerStore.byHost(last)?.let { Session.enter(it) }
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
        null -> when {
            current != null -> MainScaffold()
            else -> ServersScreen(
                onEnter = { s ->
                    android.util.Log.d("AZAPP", "card click token="+(ServerStore.token(s.host)!=null)+" host="+s.host)
                    // 未登录的服务器先进登录页（预填地址与入口码），登录成功后再进入主界面
                    if (ServerStore.token(s.host) != null) Session.enter(s)
                    else Router.push(Router.Route.Login(s, "", onLoggedIn = { Session.enter(it) }, onClose = { Router.pop() }))
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
    val tabs = listOf(
        "仪表盘" to Icons.Outlined.Speed,
        "网站" to Icons.Outlined.Language,
        "商店" to Icons.Outlined.Storefront,
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
                1 -> SitesScreen(server)
                2 -> AppStoreScreen(server)
                3 -> FilesScreen(server)
                else -> MeScreen(server)
            }
        }
    }
}
