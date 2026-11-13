// BindLoginScreen.kt —— 绑定码扫码后的免密登录执行页：自动 bind-login → 进入主界面。
package cn.hanyuxin.panel.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.Api
import cn.hanyuxin.panel.data.ApiException
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.ServerStore
import cn.hanyuxin.panel.ui.Session
import cn.hanyuxin.panel.data.clientFor
import kotlinx.coroutines.launch
import android.os.Build

/** 绑定码免密登录：自动用绑定密钥换取会话并进入主界面。 */
@Composable
fun BindLoginScreen(server: ServerInfo, bindKey: String, onClose: () -> Unit) {
    val scope = rememberCoroutineScope()
    var phase by remember { mutableStateOf("正在绑定…") }
    var err by remember { mutableStateOf<String?>(null) }
    var done by remember { mutableStateOf(false) }

    suspend fun runBind() {
        scope.launch {
            try {
                val device = "${Build.MANUFACTURER} ${Build.MODEL}".trim()
                val resp = Api.bindLogin(server, bindKey, device)
                phase = "绑定成功，正在进入面板…"
                kotlinx.coroutines.delay(600)
                Session.enter(ServerStore.byHost(server.host) ?: server)
                onClose()
            } catch (e: ApiException) {
                err = e.message
            } catch (e: Exception) {
                err = (e::class.simpleName ?: "Error") + ": " + (e.message ?: "无详情")
            }
        }
    }

    LaunchedEffect(Unit) { runBind() }

    Column(
        Modifier.fillMaxSize().padding(24.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text("绑定 APP", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        Spacer(Modifier.height(10.dp))
        Text(server.host, fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace, fontSize = 13.sp)
        Spacer(Modifier.height(20.dp))
        if (err == null) {
            CircularProgressIndicator()
            Spacer(Modifier.height(12.dp))
            Text(phase, color = MaterialTheme.colorScheme.onSurfaceVariant)
        } else {
            Text(err!!, color = MaterialTheme.colorScheme.error)
            Spacer(Modifier.height(12.dp))
            Button(onClick = { err = null; scope.launch { runBind() } }) { Text("重试") }
            Button(onClick = onClose) { Text("取消") }
        }
    }
}
