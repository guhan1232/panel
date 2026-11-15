// AuthScreens.kt —— 服务器列表、手动添加、扫码、登录、扫码授权确认（docs/APP.md §5/§6.1）。
package cn.hanyuxin.panel.ui

import android.Manifest
import android.content.pm.PackageManager
import android.os.Build
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.ImageProxy
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.camera.view.PreviewView
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.Key
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.QrCodeScanner
import androidx.compose.material.icons.outlined.Shield
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.sp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.content.ContextCompat
import cn.hanyuxin.panel.data.Api
import cn.hanyuxin.panel.data.ApiException
import cn.hanyuxin.panel.data.QrData
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.ServerStore
import cn.hanyuxin.panel.data.clientFor
import com.google.zxing.BarcodeFormat
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.MultiFormatReader
import com.google.zxing.PlanarYUVLuminanceSource
import com.google.zxing.common.HybridBinarizer
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import java.util.concurrent.Executors
import kotlin.coroutines.resume

// ---- 服务器列表 ----

@Composable
fun ServersScreen(onEnter: (ServerInfo) -> Unit, onAdd: () -> Unit, onScan: () -> Unit) {
    val servers by ServerStore.servers.collectAsState()
    var deleting by remember { mutableStateOf<ServerInfo?>(null) }

    Column(Modifier.fillMaxSize().padding(16.dp)) {
        Text("寒语馨面板", style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.Bold)
        Text("选择要管理的服务器", color = MaterialTheme.colorScheme.onSurfaceVariant, fontSize = 14.sp)
        Spacer(Modifier.height(16.dp))
        if (servers.isEmpty()) {
            EmptyView("还没有添加服务器\n推荐用面板 Web 登录页的「扫码登录」二维码一键添加")
        } else {
            LazyColumn(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(10.dp)) {
                items(servers, key = { it.host }) { s ->
                    Card(
                        shape = RoundedCornerShape(14.dp),
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                        modifier = Modifier.fillMaxWidth().clickable { onEnter(s) },
                    ) {
                        Row(Modifier.padding(14.dp), verticalAlignment = Alignment.CenterVertically) {
                            Column(Modifier.weight(1f)) {
                                Text(s.name.ifBlank { s.host }, fontWeight = FontWeight.SemiBold)
                                Text(s.host, fontSize = 12.sp, fontFamily = FontFamily.Monospace, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                Row(verticalAlignment = Alignment.CenterVertically) {
                                    Icon(Icons.Outlined.Key, null, Modifier.size(13.dp), tint = MaterialTheme.colorScheme.onSurfaceVariant)
                                    Text(
                                        if (ServerStore.token(s.host) != null) " 已登录" else " 未登录（点击后登录）",
                                        fontSize = 12.sp,
                                        color = if (ServerStore.token(s.host) != null) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                                    )
                                }
                            }
                            var menu by remember { mutableStateOf(false) }
                            Box {
                                Icon(Icons.Filled.MoreVert, "更多", Modifier.clickable { menu = true }, tint = MaterialTheme.colorScheme.onSurfaceVariant)
                                DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                                    DropdownMenuItem(text = { Text("删除") }, leadingIcon = { Icon(Icons.Outlined.Delete, null) }, onClick = { menu = false; deleting = s })
                                }
                            }
                        }
                    }
                }
            }
        }
        Spacer(Modifier.height(12.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            Button(onClick = onScan, modifier = Modifier.weight(1f)) {
                Icon(Icons.Outlined.QrCodeScanner, null, Modifier.size(18.dp))
                Spacer(Modifier.width(6.dp))
                Text("扫码添加")
            }
            OutlinedButton(onClick = onAdd, modifier = Modifier.weight(1f)) {
                Icon(Icons.Outlined.Add, null, Modifier.size(18.dp))
                Spacer(Modifier.width(6.dp))
                Text("手动添加")
            }
        }
    }
    deleting?.let { s ->
        AlertDialog(
            onDismissRequest = { deleting = null },
            title = { Text("删除服务器") },
            text = { Text("将删除 ${s.host} 的本地记录与会话令牌（不影响面板本身）。") },
            confirmButton = { TextButton(onClick = { ServerStore.remove(s.host); deleting = null }) { Text("删除") } },
            dismissButton = { TextButton(onClick = { deleting = null }) { Text("取消") } },
        )
    }
}

// ---- 手动添加 ----

@Composable
fun AddServerScreen(onDone: () -> Unit) {
    var host by remember { mutableStateOf("") }
    var entrance by remember { mutableStateOf("") }
    var name by remember { mutableStateOf("") }
    var err by remember { mutableStateOf<String?>(null) }

    Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text("手动添加服务器", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        Text("面板地址形如 https://IP:端口；入口码见安装完成提示或 Web 登录页地址 /s/<入口码>", fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
        OutlinedTextField(host, { host = it }, label = { Text("面板地址") }, placeholder = { Text("https://IP:端口，如 https://1.2.3.4:27958") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
        OutlinedTextField(entrance, { entrance = it }, label = { Text("安全入口码") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
        OutlinedTextField(name, { name = it }, label = { Text("备注名（可选）") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
        ErrorBanner(err)
        Button(
            onClick = {
                val h = host.trim()
                if (!h.startsWith("http://") && !h.startsWith("https://")) {
                    err = "面板地址需以 http(s):// 开头"
                    return@Button
                }
                if (entrance.isBlank()) {
                    err = "请填写安全入口码"
                    return@Button
                }
                ServerStore.upsert(h, entrance.trim(), name.trim())
                onDone()
            },
            modifier = Modifier.fillMaxWidth(),
        ) { Text("保存") }
        OutlinedButton(onClick = onDone, modifier = Modifier.fillMaxWidth()) { Text("取消") }
    }
}

// ---- 扫码（CameraX + zxing，无 GMS 依赖）----

@Composable
fun ScannerScreen(onQr: (QrData) -> Unit, onClose: () -> Unit) {
    val ctx = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current
    var hasPermission by remember {
        mutableStateOf(ContextCompat.checkSelfPermission(ctx, Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED)
    }
    val permLauncher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted -> hasPermission = granted }
    LaunchedEffect(Unit) { if (!hasPermission) permLauncher.launch(Manifest.permission.CAMERA) }

    val previewView = remember { PreviewView(ctx) }
    val analyzerExecutor = remember { Executors.newSingleThreadExecutor() }
    var result by remember { mutableStateOf<QrData?>(null) }
    DisposableEffect(Unit) { onDispose { analyzerExecutor.shutdown() } }

    Box(Modifier.fillMaxSize()) {
        if (hasPermission) {
            AndroidView(factory = { previewView }, modifier = Modifier.fillMaxSize())
            LaunchedEffect(Unit) {
                val provider = suspendCancellableCoroutine { cont ->
                    val fut = ProcessCameraProvider.getInstance(ctx)
                    fut.addListener({ cont.resume(fut.get()) }, ContextCompat.getMainExecutor(ctx))
                }
                val preview = Preview.Builder().build().also { it.setSurfaceProvider(previewView.surfaceProvider) }
                val reader = MultiFormatReader().apply {
                    setHints(
                        mapOf(
                            DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE),
                            DecodeHintType.TRY_HARDER to true,
                        ),
                    )
                }
                val analysis = ImageAnalysis.Builder()
                    .setTargetResolution(android.util.Size(1280, 720))
                    .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
                    .build()
                analysis.setAnalyzer(analyzerExecutor) { proxy ->
                    if (result == null) {
                        decodeFrame(reader, proxy)?.let { text ->
                            QrData.parse(text)?.let { qr ->
                                result = qr
                                ContextCompat.getMainExecutor(ctx).execute { onQr(qr) }
                            }
                        }
                    }
                    proxy.close()
                }
                provider.unbindAll()
                val camera = provider.bindToLifecycle(lifecycleOwner, CameraSelector.DEFAULT_BACK_CAMERA, preview, analysis)
                // 拍屏幕二维码需要近距离对焦：对取景中心持续自动对焦（disableAutoCancel = 不自动解除）
                camera.cameraControl.startFocusAndMetering(
                    androidx.camera.core.FocusMeteringAction.Builder(
                        androidx.camera.core.SurfaceOrientedMeteringPointFactory(1f, 1f).createPoint(0.5f, 0.5f),
                        androidx.camera.core.FocusMeteringAction.FLAG_AF or androidx.camera.core.FocusMeteringAction.FLAG_AE,
                    ).disableAutoCancel().build()
                )
            }
        } else {
            Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.Center, horizontalAlignment = Alignment.CenterHorizontally) {
                Text("需要相机权限才能扫码", color = MaterialTheme.colorScheme.onSurfaceVariant)
                Spacer(Modifier.height(12.dp))
                Button(onClick = { permLauncher.launch(Manifest.permission.CAMERA) }) { Text("授权相机") }
            }
        }
        Row(Modifier.align(Alignment.TopStart).padding(8.dp)) {
            OutlinedButton(onClick = onClose) { Text("关闭") }
        }
        Text(
            "将二维码对准取景框 · 保持 10–30cm · 光线充足更易识别",
            color = androidx.compose.ui.graphics.Color.White,
            fontSize = 13.sp,
            modifier = Modifier.align(Alignment.BottomCenter).padding(bottom = 28.dp)
                .background(androidx.compose.ui.graphics.Color.Black.copy(alpha = 0.55f), RoundedCornerShape(8.dp))
                .padding(horizontal = 14.dp, vertical = 8.dp),
        )
    }
}

/** YUV → 亮度平面 → zxing 解码，尝试四个旋转角（竖屏 QR 兼容）。 */
private fun decodeFrame(reader: MultiFormatReader, image: ImageProxy): String? {
    return try {
        val plane = image.planes[0]
        val buf = plane.buffer
        val w = image.width
        val h = image.height
        val rowStride = plane.rowStride
        val pixelStride = plane.pixelStride
        val data = ByteArray(w * h)
        if (pixelStride == 1 && rowStride == w) {
            buf.get(data)
        } else {
            var pos = 0
            for (row in 0 until h) {
                buf.position(row * rowStride)
                if (pixelStride == 1) {
                    buf.get(data, pos, w)
                } else {
                    for (col in 0 until w) data[pos + col] = buf.get(row * rowStride + col * pixelStride)
                }
                pos += w
            }
        }
        buf.rewind()
        var bitmap = BinaryBitmap(HybridBinarizer(PlanarYUVLuminanceSource(data, w, h, 0, 0, w, h, false)))
        repeat(4) {
            try {
                return reader.decode(bitmap)?.text
            } catch (_: Exception) {
                bitmap = bitmap.rotateCounterClockwise()
            }
        }
        null
    } catch (_: Exception) {
        null
    } finally {
        reader.reset()
    }
}

// ---- 登录（账密 + 2FA；带 qrKey 时登录后自动为浏览器授权）----

@Composable
fun LoginScreen(server: ServerInfo, qrKey: String, onLoggedIn: (ServerInfo) -> Unit, onClose: () -> Unit) {
    val scope = rememberCoroutineScope()
    var user by remember { mutableStateOf("") }
    var pass by remember { mutableStateOf("") }
    var code by remember { mutableStateOf("") }
    var need2fa by remember { mutableStateOf(false) }
    var busy by remember { mutableStateOf(false) }
    var err by remember { mutableStateOf<String?>(null) }
    var status by remember { mutableStateOf<String?>(null) }

    Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
        Text("登录 ${server.name.ifBlank { server.host }}", style = MaterialTheme.typography.headlineSmall, fontWeight = FontWeight.Bold)
        Text(
            "入口码 ${server.entrance}",
            fontSize = 12.sp,
            fontFamily = FontFamily.Monospace,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        OutlinedTextField(user, { user = it }, label = { Text("用户名") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
        OutlinedTextField(
            pass, { pass = it },
            label = { Text("密码") },
            modifier = Modifier.fillMaxWidth(),
            singleLine = true,
            visualTransformation = PasswordVisualTransformation(),
        )
        if (need2fa) {
            OutlinedTextField(code, { code = it }, label = { Text("两因素验证码（6 位）") }, modifier = Modifier.fillMaxWidth(), singleLine = true)
        }
        ErrorBanner(err)
        status?.let { Text(it, color = MaterialTheme.colorScheme.primary, fontSize = 13.sp) }
        Button(
            onClick = {
                if (user.isBlank() || pass.isBlank() || (need2fa && code.isBlank())) {
                    err = "请完整填写登录信息"
                    return@Button
                }
                busy = true; err = null
                scope.launch {
                    try {
                        val device = "${Build.MANUFACTURER} ${Build.MODEL}".trim()
                        val resp = Api.login(server, user.trim(), pass, code.trim(), device)
                        if (resp.need_2fa) {
                            need2fa = true
                            err = "该账号已启用两因素验证，请输入验证码"
                        } else {
                            if (qrKey.isNotBlank()) {
                                status = "登录成功，正在为浏览器授权…"
                                val c = clientFor(ServerStore.byHost(server.host) ?: server)
                                runCatching { Api.qrConfirm(c, qrKey) }
                                runCatching { Api.qrApprove(c, qrKey) }
                                status = "已确认，浏览器将自动登录"
                                kotlinx.coroutines.delay(800)
                            }
                            onLoggedIn(ServerStore.byHost(server.host) ?: server)
                            onClose()
                        }
                    } catch (e: ApiException) {
                        err = e.message
                    } catch (e: Exception) {
                        err = (e::class.simpleName ?: "Error") + ": " + (e.message ?: "无详情")
                    } finally {
                        busy = false
                    }
                }
            },
            enabled = !busy,
            modifier = Modifier.fillMaxWidth(),
        ) {
            if (busy) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp) else Text("登录")
        }
        OutlinedButton(onClick = onClose, modifier = Modifier.fillMaxWidth()) { Text("取消") }
    }
}

// ---- 扫码授权确认（已登录服务器）----

@Composable
fun QrApproveScreen(server: ServerInfo, key: String, onDone: () -> Unit) {
    val toastCtx = LocalContext.current
    val scope = rememberCoroutineScope()
    var busy by remember { mutableStateOf(false) }
    var err by remember { mutableStateOf<String?>(null) }

    Column(Modifier.fillMaxSize().padding(24.dp), verticalArrangement = Arrangement.Center, horizontalAlignment = Alignment.CenterHorizontally) {
        Card(shape = RoundedCornerShape(18.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)) {
            Column(Modifier.padding(24.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                Icon(Icons.Outlined.Shield, null, Modifier.size(44.dp), tint = MaterialTheme.colorScheme.primary)
                Spacer(Modifier.height(12.dp))
                Text("确认在浏览器登录？", style = MaterialTheme.typography.titleMedium, fontWeight = FontWeight.Bold)
                Spacer(Modifier.height(6.dp))
                Text("服务器", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Text(server.host, fontFamily = FontFamily.Monospace, fontSize = 14.sp)
                Spacer(Modifier.height(8.dp))
                Text("确认后，扫过该二维码的浏览器将免密登录该面板。请核对服务器地址无误。", fontSize = 12.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
                Spacer(Modifier.height(16.dp))
                ErrorBanner(err)
                if (busy) {
                    CircularProgressIndicator()
                } else {
                    Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                        OutlinedButton(onClick = onDone) { Text("取消") }
                        Button(onClick = {
                            busy = true; err = null
                            scope.launch {
                                try {
                                    val c = clientFor(server)
                                    Api.qrConfirm(c, key)
                                    Api.qrApprove(c, key)
                                    android.widget.Toast.makeText(toastCtx, "已在浏览器完成登录", android.widget.Toast.LENGTH_SHORT).show()
                                    onDone()
                                } catch (e: Exception) {
                                    err = e.message ?: "授权失败"
                                } finally {
                                    busy = false
                                }
                            }
                        }) { Text("确认登录") }
                    }
                }
            }
        }
    }
}
