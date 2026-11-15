// TerminalScreen.kt —— 服务器终端：WebSocket 连 /api/ws/terminal（Bearer 鉴权），行模式交互。
// PTY 输出为文本帧（做轻量 ANSI 清洗），输入按 wsMsg JSON 帧（{type:"input",data:"cmd\n"}）发送。
package cn.hanyuxin.panel.ui

import androidx.compose.foundation.background
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
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
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
import cn.hanyuxin.panel.data.Http
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.ServerStore
import kotlinx.coroutines.launch
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString

/** 去除 ANSI 转义（CSI/OSC/其他单字符序列），保留可见文本。 */
private val ansiRegex = Regex("\u001B\\[[0-9;?]*[a-zA-Z]|\u001B\\][^\u0007]*\u0007|\u001B[@-Z\\\\-_]")

private fun cleanAnsi(s: String) = s.replace(ansiRegex, "").replace("\r", "")

@Composable
fun TerminalScreen(server: ServerInfo) {
    val scope = rememberCoroutineScope()
    val output = remember { mutableStateListOf<String>() } // 逐块输出（已清洗）
    var status by remember { mutableStateOf("连接中…") }
    var input by remember { mutableStateOf("") }
    val history = remember { mutableStateListOf<String>() }
    var ws by remember { mutableStateOf<WebSocket?>(null) }
    val listState = rememberLazyListState()

    DisposableEffect(server.host) {
        val token = ServerStore.token(server.host)
        val url = server.host.trimEnd('/') + "/api/ws/terminal?cols=110&rows=30"
        val req = Request.Builder().url(url)
            .header("Authorization", "Bearer $token")
            .header("Cookie", "wm_entrance=${server.entrance}")
            .build()
        val client: OkHttpClient = Http.clientFor(server, token)
            .newBuilder()
            .pingInterval(java.time.Duration.ofSeconds(30))
            .readTimeout(java.time.Duration.ofSeconds(0)) // WS 常连不超时
            .build()
        var closed = false
        val socket = client.newWebSocket(req, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                status = "已连接（本机）"
                ws = webSocket
            }
            override fun onMessage(webSocket: WebSocket, text: String) {
                // JSON 帧（connected/exit）或 PTY 原始输出
                val t = runCatching {
                    val e = Http.json.parseToJsonElement(text).takeIf { it is kotlinx.serialization.json.JsonObject }
                    ((e as? kotlinx.serialization.json.JsonObject)?.get("type") as? kotlinx.serialization.json.JsonPrimitive)?.content
                }.getOrNull()
                when (t) {
                    "exit" -> {
                        val msg = (runCatching { Http.json.parseToJsonElement(text).jsonObject["data"] }.getOrNull() as? kotlinx.serialization.json.JsonPrimitive)?.content ?: "连接已断开"
                        status = "已断开：$msg"
                    }
                    "connected", "idle" -> {}
                    else -> output.add(cleanAnsi(text))
                }
            }
            override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
                output.add(cleanAnsi(bytes.utf8()))
            }
            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                if (!closed) status = "连接失败：${t.message ?: "未知错误"}"
            }
            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                if (!closed) status = "已断开"
            }
        })
        ws = socket
        onDispose {
            closed = true
            runCatching { socket.close(1000, "bye") }
        }
    }

    fun send(cmd: String) {
        val w = ws ?: return
        w.send(buildJsonObject {
            put("type", kotlinx.serialization.json.JsonPrimitive("input"))
            put("data", kotlinx.serialization.json.JsonPrimitive(cmd + "\n"))
        }.toString())
        history.add(cmd)
    }

    Column(Modifier.fillMaxSize().padding(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("终端", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            Text(status, fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Spacer(Modifier.height(8.dp))
        Column(
            Modifier.fillMaxWidth().weight(1f).background(MaterialTheme.colorScheme.surface, RoundedCornerShape(12.dp)).padding(10.dp),
        ) {
            LazyColumn(state = listState, modifier = Modifier.weight(1f)) {
                items(output) { chunk ->
                    Text(chunk, fontSize = 11.sp, fontFamily = FontFamily.Monospace)
                }
            }
            androidx.compose.runtime.LaunchedEffect(output.size) {
                if (output.isNotEmpty()) listState.animateScrollToItem(output.size - 1)
            }
        }
        Spacer(Modifier.height(8.dp))
        if (history.isNotEmpty()) {
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.padding(bottom = 6.dp)) {
                history.takeLast(4).reversed().forEach { h ->
                    OutlinedButton(onClick = { send(h) }, enabled = status == "已连接（本机）",
                        contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 8.dp, vertical = 2.dp)) {
                        Text(h.take(16), fontSize = 11.sp, fontFamily = FontFamily.Monospace)
                    }
                }
            }
        }
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            OutlinedButton(
                enabled = status == "已连接（本机）",
                onClick = {
                    ws?.send(buildJsonObject {
                        put("type", kotlinx.serialization.json.JsonPrimitive("input"))
                        put("data", kotlinx.serialization.json.JsonPrimitive("\u0003"))
                    }.toString())
                },
                contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 12.dp, vertical = 6.dp),
            ) { Text("Ctrl+C", fontSize = 12.sp) }
            OutlinedTextField(
                value = input,
                onValueChange = { input = it },
                modifier = Modifier.weight(1f),
                placeholder = { Text("输入命令…", fontSize = 13.sp) },
                singleLine = true,
                textStyle = androidx.compose.ui.text.TextStyle(fontFamily = FontFamily.Monospace, fontSize = 13.sp),
            )
            TextButton(
                enabled = status == "已连接（本机）" && input.isNotBlank(),
                onClick = {
                    send(input)
                    input = ""
                },
            ) { Text("执行") }
        }
    }
}
