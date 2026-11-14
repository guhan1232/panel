// TerminalScreen.kt —— 服务器终端：WebSocket 连 /api/ws/terminal（本机）或 /api/ws/ssh?host_id=（SSH 主机，面板代理解密凭据）。
// 深色终端区 + 键盘工具条（ESC/TAB/CTRL 系列/方向键）+ 历史命令回填 + 主机切换 + 断线重连。
// 输入为 JSON 帧（{type:"input",data:"cmd\n"}），PTY 输出为文本帧（轻量 ANSI 清洗）。
package cn.hanyuxin.panel.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material3.AssistChip
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.Http
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.ServerStore
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.put
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import okio.ByteString

/** 去除 ANSI 转义（CSI/OSC/其他单字符序列），保留可见文本。 */
private val ansiRegex = Regex("\\[[0-9;?]*[a-zA-Z]][^]*|[@-Z\\\\-_]")

private fun cleanAnsi(s: String) = s.replace(ansiRegex, "").replace("\r", "")

/** 终端深色配色（终端惯例：深底浅字）。 */
private val TermBg = Color(0xFF0A1020)
private val TermFg = Color(0xFFD8E2F0)
private val TermDim = Color(0xFF6B7A94)

/** 键盘工具条发送的控制序列（Kotlin 转义）。 */
private const val KEY_ESC = ""
private const val KEY_TAB = "	"
private const val KEY_CTRL_C = ""
private const val KEY_CTRL_D = ""
private const val KEY_CTRL_L = ""
private const val KEY_CTRL_U = ""

/** SSH 主机收藏（面板 /api/term/hosts，凭据面板侧解密代理连接）。 */
@kotlinx.serialization.Serializable
private data class TermHostRef(val id: Long, val name: String, val host: String)

@Composable
fun TerminalScreen(server: ServerInfo) {
    val output = remember { mutableStateListOf<String>() } // 逐块输出（已清洗）
    var status by remember { mutableStateOf("连接中…") }
    var connected by remember { mutableStateOf(false) }
    var input by remember { mutableStateOf("") }
    val history = remember { mutableStateListOf<String>() }
    var histIdx by remember { mutableIntStateOf(-1) } // -1 = 当前输入
    var ws by remember { mutableStateOf<WebSocket?>(null) }
    var hosts by remember { mutableStateOf<List<TermHostRef>>(emptyList()) }
    var target by remember { mutableStateOf("local") } // "local" | "ssh:<hostId>"
    var reconnectTick by remember { mutableIntStateOf(0) } // 重连触发器
    val termState = rememberLazyListState()

    // SSH 主机收藏列表（面板 /api/term/hosts）
    LaunchedEffect(server.host) {
        runCatching {
            val pc = cn.hanyuxin.panel.data.PanelClient(server, ServerStore.token(server.host))
            hosts = pc.request<List<TermHostRef>>("GET", "/api/term/hosts") ?: emptyList()
        }
    }

    fun sendRaw(data: String) {
        ws?.send(buildJsonObject {
            put("type", JsonPrimitive("input"))
            put("data", JsonPrimitive(data))
        }.toString())
    }

    fun disconnect() {
        runCatching { ws?.close(1000, "bye") }
        ws = null
        connected = false
    }

    fun send(cmd: String) {
        val w = ws ?: return
        w.send(buildJsonObject {
            put("type", JsonPrimitive("input"))
            put("data", JsonPrimitive(cmd + "\n"))
        }.toString())
        if (cmd.isNotBlank()) {
            history.add(cmd)
            histIdx = -1
        }
    }

    // 历史命令 上/下 回填
    fun histMove(dir: Int) {
        if (history.isEmpty()) return
        histIdx = (histIdx + dir).coerceIn(-1, history.size - 1)
        input = if (histIdx < 0) "" else history[histIdx]
    }

    DisposableEffect(target, server.host, reconnectTick) {
        var closed = false
        val isLocal = target == "local"
        val hostId = target.removePrefix("ssh:").toLongOrNull() ?: 0L
        val token = ServerStore.token(server.host)
        val url = if (isLocal) {
            server.host.trimEnd('/') + "/api/ws/terminal?cols=110&rows=30"
        } else {
            server.host.trimEnd('/') + "/api/ws/ssh?host_id=$hostId&cols=110&rows=30"
        }
        val req = Request.Builder().url(url)
            .header("Authorization", "Bearer $token")
            .header("Cookie", "wm_entrance=${server.entrance}")
            .build()
        val client: OkHttpClient = Http.clientFor(server, token)
            .newBuilder()
            .pingInterval(java.time.Duration.ofSeconds(30))
            .readTimeout(java.time.Duration.ofSeconds(0)) // WS 常连不超时
            .build()
        output.clear()
        val socket = client.newWebSocket(req, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                status = if (isLocal) "已连接（本机）" else "已连接（SSH）"
                connected = true
                ws = webSocket
            }
            override fun onMessage(webSocket: WebSocket, text: String) {
                val t = runCatching {
                    val e = Http.json.parseToJsonElement(text).takeIf { it is kotlinx.serialization.json.JsonObject }
                    ((e as? kotlinx.serialization.json.JsonObject)?.get("type") as? JsonPrimitive)?.content
                }.getOrNull()
                when (t) {
                    "exit" -> {
                        val msg = (runCatching { Http.json.parseToJsonElement(text).jsonObject["data"] }.getOrNull() as? JsonPrimitive)?.content ?: "连接已断开"
                        status = "已断开：$msg"
                        connected = false
                    }
                    "connected", "idle" -> {}
                    else -> output.add(cleanAnsi(text))
                }
            }
            override fun onMessage(webSocket: WebSocket, bytes: ByteString) {
                output.add(cleanAnsi(bytes.utf8()))
            }
            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                if (!closed) {
                    status = "连接失败：${t.message ?: "未知错误"}"
                    connected = false
                }
            }
            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                if (!closed) {
                    status = "已断开"
                    connected = false
                }
            }
        })
        ws = socket
        onDispose {
            closed = true
            runCatching { socket.close(1000, "bye") }
            ws = null
        }
    }

    Column(Modifier.fillMaxSize().padding(16.dp)) {
        // ---- 标题行：状态 pill + 重连 ----
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("终端", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            Surface(shape = RoundedCornerShape(8.dp), color = if (connected) Color(0xFF16A34A).copy(alpha = 0.12f) else MaterialTheme.colorScheme.error.copy(alpha = 0.10f)) {
                Text(
                    status, fontSize = 11.sp, fontWeight = FontWeight.Medium,
                    color = if (connected) Color(0xFF16A34A) else MaterialTheme.colorScheme.error,
                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
                )
            }
            Spacer(Modifier.width(6.dp))
            if (!connected) {
                IconButton(onClick = { reconnectTick++ }) {
                    Icon(Icons.Outlined.Refresh, "重连", modifier = Modifier.size(20.dp))
                }
            }
        }
        Spacer(Modifier.height(8.dp))

        // ---- 连接目标 chips：本机 / SSH 主机收藏 ----
        Row(
            Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
        ) {
            FilterChip(
                selected = target == "local",
                onClick = { if (target != "local") { disconnect(); target = "local" } },
                label = { Text("本机面板", fontSize = 12.sp) },
            )
            hosts.forEach { h ->
                FilterChip(
                    selected = target == "ssh:" + h.id,
                    onClick = { if (target != "ssh:" + h.id) { disconnect(); target = "ssh:" + h.id } },
                    label = { Text(h.name.ifBlank { h.host }, fontSize = 12.sp) },
                )
            }
            if (hosts.isEmpty()) {
                Text(
                    "暂无 SSH 主机收藏（面板 → 终端 → 主机管理添加）",
                    fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(top = 10.dp),
                )
            }
        }
        Spacer(Modifier.height(8.dp))

        // ---- 终端输出区（深色）----
        Box(
            Modifier.fillMaxWidth().weight(1f).background(TermBg, RoundedCornerShape(12.dp)).padding(10.dp),
        ) {
            LazyColumn(state = termState, modifier = Modifier.fillMaxSize()) {
                items(output) { chunk ->
                    Text(chunk, fontSize = 12.sp, fontFamily = FontFamily.Monospace, color = TermFg)
                }
                if (output.isEmpty()) {
                    item {
                        Text(
                            if (connected) "已连接。输入命令开始…" else status,
                            fontSize = 12.sp, fontFamily = FontFamily.Monospace,
                            color = TermDim,
                        )
                    }
                }
            }
            LaunchedEffect(output.size) {
                if (output.isNotEmpty()) termState.animateScrollToItem(output.size - 1)
            }
        }
        Spacer(Modifier.height(8.dp))

        // ---- 历史命令快捷按钮 ----
        if (history.isNotEmpty()) {
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp), modifier = Modifier.padding(bottom = 6.dp)) {
                history.takeLast(4).reversed().forEach { h ->
                    AssistChip(
                        onClick = { input = h; histIdx = history.indexOf(h) },
                        label = { Text(h.take(16), fontSize = 11.sp, fontFamily = FontFamily.Monospace) },
                    )
                }
            }
        }

        // ---- 键盘工具条：ESC / TAB / CTRL 系列 / 常用符号 / 历史方向 ----
        Row(
            Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()),
            horizontalArrangement = Arrangement.spacedBy(4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            listOf(
                "ESC" to KEY_ESC,
                "TAB" to KEY_TAB,
                "Ctrl+C" to KEY_CTRL_C,
                "Ctrl+D" to KEY_CTRL_D,
                "Ctrl+L" to KEY_CTRL_L,
                "Ctrl+U" to KEY_CTRL_U,
                "|" to "|",
                "~" to "~",
                "-" to "-",
                "/" to "/",
            ).forEach { (label, seq) ->
                AssistChip(
                    onClick = { sendRaw(seq) },
                    enabled = connected,
                    label = { Text(label, fontSize = 11.sp, fontFamily = FontFamily.Monospace) },
                )
            }
            TextButton(onClick = { histMove(-1) }, enabled = history.isNotEmpty(), contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 6.dp)) {
                Text("↑", fontSize = 14.sp, fontWeight = FontWeight.Bold)
            }
            TextButton(onClick = { histMove(1) }, enabled = history.isNotEmpty() && histIdx >= 0, contentPadding = androidx.compose.foundation.layout.PaddingValues(horizontal = 6.dp)) {
                Text("↓", fontSize = 14.sp, fontWeight = FontWeight.Bold)
            }
        }
        Spacer(Modifier.height(6.dp))

        // ---- 输入行 ----
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            AssistChip(
                onClick = { sendRaw(KEY_CTRL_C) },
                enabled = connected,
                label = { Text("Ctrl+C", fontSize = 12.sp) },
            )
            OutlinedTextField(
                value = input,
                onValueChange = { input = it },
                modifier = Modifier.weight(1f),
                placeholder = { Text("输入命令…", fontSize = 13.sp) },
                singleLine = true,
                textStyle = androidx.compose.ui.text.TextStyle(fontFamily = FontFamily.Monospace, fontSize = 13.sp, color = if (connected) TermFg else MaterialTheme.colorScheme.onSurface),
            )
            TextButton(
                enabled = connected && input.isNotBlank(),
                onClick = {
                    send(input)
                    input = ""
                    histIdx = -1
                },
            ) { Text("执行", fontWeight = FontWeight.Bold) }
        }
    }
}
