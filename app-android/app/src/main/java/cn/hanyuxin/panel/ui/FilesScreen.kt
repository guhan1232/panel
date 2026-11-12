// FilesScreen.kt —— 文件管理：浏览、mkdir/重命名/删除、上传下载、文本编辑（docs/APP.md §6.5）。
package cn.hanyuxin.panel.ui

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
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
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.CreateNewFolder
import androidx.compose.material.icons.outlined.Description
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.MoreVert
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material.icons.outlined.CloudUpload
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.Api
import cn.hanyuxin.panel.data.ApiException
import cn.hanyuxin.panel.data.FileEntry
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.clientFor
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.time.OffsetDateTime
import java.time.format.DateTimeFormatter

@Composable
fun FilesScreen(server: ServerInfo) {
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()
    var path by remember(server.host) { mutableStateOf("/") }
    var listing by remember(path) { mutableStateOf<cn.hanyuxin.panel.data.FileList?>(null) }
    var err by remember { mutableStateOf<String?>(null) }
    var reload by remember { mutableStateOf(0) }
    var busy by remember { mutableStateOf(false) }
    var dlgMkdir by remember { mutableStateOf(false) }
    var dlgRename by remember { mutableStateOf<FileEntry?>(null) }
    var dlgDelete by remember { mutableStateOf<FileEntry?>(null) }
    var menuFor by remember { mutableStateOf<FileEntry?>(null) }
    var uploading by remember { mutableStateOf(false) }
    var downloading by remember { mutableStateOf<String?>(null) }

    fun toast(s: String) = android.widget.Toast.makeText(ctx, s, android.widget.Toast.LENGTH_SHORT).show()

    LaunchedEffect(server.host, path, reload) {
        apiGuard(onError = { err = it.message }) { Api.listFiles(clientFor(server), path) }?.let { listing = it; err = null }
    }

    val uploadLauncher = rememberLauncherForActivityResult(ActivityResultContracts.GetContent()) { uri ->
        if (uri == null) return@rememberLauncherForActivityResult
        uploading = true
        scope.launch {
            try {
                val tmp = withContext(Dispatchers.IO) {
                    val f = java.io.File(ctx.cacheDir, "up-" + System.currentTimeMillis() + "-" + (uri.lastPathSegment?.substringAfterLast('/') ?: "file"))
                    ctx.contentResolver.openInputStream(uri)!!.use { input -> f.outputStream().use { input.copyTo(it) } }
                    f
                }
                Api.uploadFile(clientFor(server), path, tmp)
                tmp.delete()
                toast("上传完成")
                reload++
            } catch (e: Exception) {
                toast("上传失败：${e.message}")
            } finally { uploading = false }
        }
    }

    var downloadTarget by remember { mutableStateOf<FileEntry?>(null) }
    val downloadLauncher = rememberLauncherForActivityResult(ActivityResultContracts.CreateDocument("application/octet-stream")) { uri ->
        val entry = downloadTarget
        if (uri == null || entry == null) return@rememberLauncherForActivityResult
        downloading = entry.name
        scope.launch {
            try {
                withContext(Dispatchers.IO) {
                    ctx.contentResolver.openOutputStream(uri)!!.let { out ->
                        Api.downloadFile(clientFor(server), entry.path, out)
                        out.close()
                    }
                }
                toast("下载完成")
            } catch (e: Exception) {
                toast("下载失败：${e.message}")
            } finally { downloading = null; downloadTarget = null }
        }
    }

    Column(Modifier.fillMaxSize().padding(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = { path = parentOf(path) }, enabled = path != "/") {
                Icon(Icons.AutoMirrored.Outlined.ArrowBack, "上级")
            }
            Text(path, fontSize = 13.sp, fontFamily = FontFamily.Monospace, modifier = Modifier.weight(1f), maxLines = 2)
            if (busy || uploading) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
            IconButton(onClick = { reload++ }) { Icon(Icons.Outlined.Refresh, "刷新") }
            IconButton(onClick = { dlgMkdir = true }) { Icon(Icons.Outlined.CreateNewFolder, "新建目录") }
            IconButton(onClick = { uploadLauncher.launch("*/*") }, enabled = !uploading) { Icon(Icons.Outlined.CloudUpload, "上传") }
        }
        Spacer(Modifier.height(8.dp))
        ErrorBanner(err)
        val l = listing
        when {
            l == null -> LoadingView()
            l.items.isEmpty() -> EmptyView("空目录")
            else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                items(l.items, key = { it.path }) { e ->
                    Card(
                        shape = RoundedCornerShape(10.dp),
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface),
                        modifier = Modifier.fillMaxWidth().clickable {
                            if (e.is_dir) path = e.path else menuFor = e
                        },
                    ) {
                        Row(Modifier.padding(horizontal = 14.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                            Icon(
                                if (e.is_dir) Icons.Outlined.Folder else Icons.Outlined.Description, null,
                                tint = if (e.is_dir) MaterialTheme.colorScheme.secondary else MaterialTheme.colorScheme.onSurfaceVariant,
                                modifier = Modifier.size(20.dp),
                            )
                            Spacer(Modifier.width(10.dp))
                            Column(Modifier.weight(1f)) {
                                Text(e.name, fontSize = 14.sp, maxLines = 1)
                                Text(
                                    if (e.is_dir) "目录 · ${e.mode}" else "${humanSize(e.size)} · ${formatTime(e.mtime)}",
                                    fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                            }
                            if (!e.is_dir) {
                                Box {
                                    Icon(Icons.Outlined.MoreVert, "操作", Modifier.clickable { menuFor = e })
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    // 文件操作菜单
    menuFor?.let { e ->
        AlertDialog(
            onDismissRequest = { menuFor = null },
            title = { Text(e.name) },
            text = { Column { KeyValue("路径", e.path, mono = true); KeyValue("大小", humanSize(e.size)) } },
            confirmButton = {
                Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                    TextButton(onClick = { menuFor = null; Router.push(Router.Route.Editor(server, e.path) { Router.pop() }) }) { Text("编辑") }
                    TextButton(onClick = { menuFor = null; downloadTarget = e; downloadLauncher.launch(e.name) }) { Text("下载") }
                    TextButton(onClick = { menuFor = null; dlgRename = e }) { Text("重命名") }
                    TextButton(onClick = { menuFor = null; dlgDelete = e }) { Text("删除", color = MaterialTheme.colorScheme.error) }
                }
            },
            dismissButton = { TextButton(onClick = { menuFor = null }) { Text("关闭") } },
        )
    }

    if (dlgMkdir) {
        var name by remember { mutableStateOf("") }
        AlertDialog(
            onDismissRequest = { dlgMkdir = false },
            title = { Text("新建目录") },
            text = { OutlinedTextField(name, { name = it }, label = { Text("目录名") }, singleLine = true) },
            confirmButton = {
                TextButton(onClick = {
                    if (name.isNotBlank()) scope.launch {
                        runCatching { Api.mkdirFile(clientFor(server), path, name.trim()) }
                            .onSuccess { toast("已创建"); dlgMkdir = false; reload++ }
                            .onFailure { err = it.message }
                    }
                }) { Text("创建") }
            },
            dismissButton = { TextButton(onClick = { dlgMkdir = false }) { Text("取消") } },
        )
    }

    dlgRename?.let { e ->
        var name by remember(e.path) { mutableStateOf(e.name) }
        AlertDialog(
            onDismissRequest = { dlgRename = null },
            title = { Text("重命名") },
            text = { OutlinedTextField(name, { name = it }, label = { Text("新名称") }, singleLine = true) },
            confirmButton = {
                TextButton(onClick = {
                    scope.launch {
                        runCatching { Api.renameFile(clientFor(server), e.path, name.trim()) }
                            .onSuccess { toast("已重命名"); dlgRename = null; reload++ }
                            .onFailure { err = it.message }
                    }
                }) { Text("确定") }
            },
            dismissButton = { TextButton(onClick = { dlgRename = null }) { Text("取消") } },
        )
    }

    dlgDelete?.let { e ->
        AlertDialog(
            onDismissRequest = { dlgDelete = null },
            title = { Text("删除 ${e.name}？") },
            text = { Text("将移入面板回收站（路径 ${e.path}）") },
            confirmButton = {
                TextButton(onClick = {
                    scope.launch {
                        runCatching { Api.deleteFiles(clientFor(server), listOf(e.path)) }
                            .onSuccess { toast("已删除"); dlgDelete = null; reload++ }
                            .onFailure { err = it.message }
                    }
                }) { Text("删除", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { dlgDelete = null }) { Text("取消") } },
        )
    }
}

@Composable
fun EditorScreen(server: ServerInfo, path: String, onClose: () -> Unit) {
    val ctx = LocalContext.current
    val scope = rememberCoroutineScope()
    var content by remember(path) { mutableStateOf<String?>(null) }
    var dirty by remember(path) { mutableStateOf(false) }
    var saving by remember { mutableStateOf(false) }
    var err by remember { mutableStateOf<String?>(null) }

    fun toast(s: String) = android.widget.Toast.makeText(ctx, s, android.widget.Toast.LENGTH_SHORT).show()

    LaunchedEffect(path) {
        apiGuard(onError = { err = it.message }) { Api.readFile(clientFor(server), path) }?.let { content = it }
    }

    Column(Modifier.fillMaxSize().padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            IconButton(onClick = onClose) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, "返回") }
            Text(path, fontSize = 12.sp, fontFamily = FontFamily.Monospace, modifier = Modifier.weight(1f), maxLines = 2)
            if (saving) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
            else TextButton(onClick = {
                val c = content ?: return@TextButton
                saving = true
                scope.launch {
                    runCatching { Api.saveFile(clientFor(server), path, c) }
                        .onSuccess { dirty = false; toast("已保存") }
                        .onFailure { err = it.message }
                    saving = false
                }
            }) { Text(if (dirty) "保存*" else "保存") }
        }
        ErrorBanner(err)
        val c = content
        if (c == null) {
            LoadingView()
        } else {
            OutlinedTextField(
                value = c,
                onValueChange = { content = it; dirty = true },
                modifier = Modifier.fillMaxSize(),
                textStyle = MaterialTheme.typography.bodySmall.copy(fontFamily = FontFamily.Monospace, lineHeight = 18.sp),
            )
        }
    }
}

private fun parentOf(path: String): String {
    if (path == "/") return "/"
    val p = path.trimEnd('/')
    val i = p.lastIndexOf('/')
    return if (i <= 0) "/" else p.substring(0, i)
}

private fun formatTime(mtime: String): String = runCatching {
    OffsetDateTime.parse(mtime).format(DateTimeFormatter.ofPattern("MM-dd HH:mm"))
}.getOrElse { mtime.take(16) }
