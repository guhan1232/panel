// DatabaseScreen.kt —— 数据库管理：MySQL/PostgreSQL 双引擎，库列表/建库/删库 + 用户/授权。
package cn.hanyuxin.panel.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Refresh
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.FilterChip
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.Api
import cn.hanyuxin.panel.data.DbStatus
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.clientFor
import kotlinx.coroutines.launch

@Composable
fun DatabaseScreen(server: ServerInfo) {
    val scope = rememberCoroutineScope()
    var engine by remember { mutableStateOf("mysql") }
    var status by remember { mutableStateOf<DbStatus?>(null) }
    var databases by remember { mutableStateOf<List<String>?>(null) }
    var users by remember { mutableStateOf<List<String>>(emptyList()) }
    var reload by remember { mutableStateOf(0) }
    var err by remember { mutableStateOf<String?>(null) }
    var busy by remember { mutableStateOf(false) }
    var createDb by remember { mutableStateOf(false) }
    var createUser by remember { mutableStateOf(false) }
    var confirmDel by remember { mutableStateOf<Pair<String, Boolean>?>(null) } // name to isDb
    var newName by remember { mutableStateOf("") }
    var newHost by remember { mutableStateOf("%") }
    var newPass by remember { mutableStateOf("") }
    var grantUser by remember { mutableStateOf<String?>(null) }
    var grantDb by remember { mutableStateOf("") }

    fun act(block: suspend () -> Unit) {
        if (busy) return
        busy = true
        scope.launch {
            try { block(); reload++ } catch (e: Exception) { err = e.message ?: "操作失败" } finally { busy = false }
        }
    }

    LaunchedEffect(server.host, reload, engine) {
        apiGuard(onError = { err = it.message }) { Api.dbStatus(clientFor(server)) }?.let { status = it }
        apiGuard(onError = { err = it.message }) { Api.dbDatabases(clientFor(server), engine) }?.let { databases = it }
        if (engine == "mysql") {
            apiGuard(onError = { err = it.message }) {
                Api.dbUsers(clientFor(server), "mysql").mapNotNull {
                    when (it) {
                        is cn.hanyuxin.panel.data.DbUser -> "${it.user}@${it.host}"
                        is String -> it
                        else -> null
                    }
                }
            }?.let { users = it }
        } else {
            apiGuard(onError = { err = it.message }) { Api.dbUsers(clientFor(server), "pg").map { it.toString() } }?.let { users = it }
        }
    }

    val installed = if (engine == "mysql") status?.mysql?.installed else status?.pg?.installed
    Column(Modifier.fillMaxSize().padding(16.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text("数据库", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold, modifier = Modifier.weight(1f))
            IconButton(onClick = { reload++ }) { Icon(Icons.Outlined.Refresh, "刷新") }
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            listOf("mysql" to "MySQL", "pg" to "PostgreSQL").forEach { (k, label) ->
                FilterChip(selected = engine == k, onClick = { engine = k; databases = null }, label = { Text(label) })
            }
        }
        Spacer(Modifier.height(8.dp))
        ErrorBanner(err)
        if (installed == false) {
            EmptyView("${if (engine == "mysql") "MySQL/MariaDB" else "PostgreSQL"} 未安装，请在软件商店安装")
            return@Column
        }
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp), modifier = Modifier.padding(vertical = 8.dp)) {
            ActionChip("创建数据库", busy) { createDb = true }
            ActionChip("创建用户", busy) { createUser = true }
        }
        val dbs = databases
        when {
            dbs == null -> LoadingView()
            else -> LazyColumn(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                items(dbs) { name ->
                    Card(shape = RoundedCornerShape(12.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)) {
                        Row(Modifier.padding(horizontal = 14.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                            Text(name, fontWeight = FontWeight.SemiBold, fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace, modifier = Modifier.weight(1f))
                            ActionChip("删除", busy, danger = true) { confirmDel = name to true }
                        }
                    }
                }
                if (users.isNotEmpty()) {
                    item {
                        Text("用户（点授权把库授权给该用户）", fontWeight = FontWeight.Bold, fontSize = 14.sp, modifier = Modifier.padding(top = 12.dp, bottom = 4.dp))
                    }
                    items(users) { u ->
                        Card(shape = RoundedCornerShape(12.dp), colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surface)) {
                            Row(Modifier.padding(horizontal = 14.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                                Text(u, fontWeight = FontWeight.SemiBold, fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace, modifier = Modifier.weight(1f))
                                ActionChip("授权", busy) { grantUser = u; grantDb = "" }
                                ActionChip("删除", busy, danger = true) { confirmDel = u to false }
                            }
                        }
                    }
                }
            }
        }
    }

    // 创建数据库
    if (createDb) {
        AlertDialog(
            onDismissRequest = { createDb = false; newName = "" },
            title = { Text("创建数据库") },
            text = { OutlinedTextField(value = newName, onValueChange = { newName = it }, label = { Text("库名") }, modifier = Modifier.fillMaxWidth()) },
            confirmButton = {
                TextButton(enabled = newName.isNotBlank() && !busy, onClick = {
                    act { Api.dbCreateDatabase(clientFor(server), engine, newName.trim()); createDb = false; newName = "" }
                }) { Text("创建") }
            },
            dismissButton = { TextButton(onClick = { createDb = false }) { Text("取消") } },
        )
    }
    // 创建用户
    if (createUser) {
        AlertDialog(
            onDismissRequest = { createUser = false; newPass = "" },
            title = { Text("创建用户") },
            text = {
                Column {
                    OutlinedTextField(value = newName, onValueChange = { newName = it }, label = { Text("用户名") }, modifier = Modifier.fillMaxWidth())
                    if (engine == "mysql") {
                        OutlinedTextField(value = newHost, onValueChange = { newHost = it }, label = { Text("主机（% 为任意）") }, modifier = Modifier.fillMaxWidth())
                    }
                    OutlinedTextField(value = newPass, onValueChange = { newPass = it }, label = { Text("密码（至少 6 位）") }, modifier = Modifier.fillMaxWidth())
                }
            },
            confirmButton = {
                TextButton(enabled = newName.isNotBlank() && newPass.length >= 6 && !busy, onClick = {
                    act {
                        Api.dbCreateUser(clientFor(server), engine, newName.trim(), newHost.trim(), newPass)
                        createUser = false; newName = ""; newPass = ""
                    }
                }) { Text("创建") }
            },
            dismissButton = { TextButton(onClick = { createUser = false; newPass = "" }) { Text("取消") } },
        )
    }
    // 授权对话框：给选中用户授权库
    grantUser?.let { u ->
        AlertDialog(
            onDismissRequest = { grantUser = null },
            title = { Text("授权 · $u") },
            text = {
                Column {
                    OutlinedTextField(value = grantDb, onValueChange = { grantDb = it }, label = { Text("库名") }, modifier = Modifier.fillMaxWidth())
                    databases?.let { dbs ->
                        Spacer(Modifier.height(6.dp))
                        Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                            dbs.take(3).forEach { d ->
                                FilterChip(selected = grantDb == d, onClick = { grantDb = d }, label = { Text(d, fontSize = 11.sp) })
                            }
                        }
                    }
                }
            },
            confirmButton = {
                TextButton(enabled = grantDb.isNotBlank() && !busy, onClick = {
                    act {
                        val user = u.substringBefore("@")
                        val host = u.substringAfter("@", "%")
                        Api.dbGrant(clientFor(server), engine, user, host, grantDb.trim())
                        grantUser = null
                    }
                }) { Text("授权") }
            },
            dismissButton = { TextButton(onClick = { grantUser = null }) { Text("取消") } },
        )
    }
    // 删除确认
    confirmDel?.let { (name, isDb) ->
        AlertDialog(
            onDismissRequest = { confirmDel = null },
            title = { Text("确认删除") },
            text = { Text(if (isDb) "将删除数据库 $name（含数据），且不可恢复。" else "将删除用户 $name。") },
            confirmButton = {
                TextButton(onClick = {
                    act {
                        if (isDb) Api.dbDeleteDatabase(clientFor(server), engine, name)
                        else {
                            if (engine == "mysql") Api.dbDeleteUserMysql(clientFor(server), name)
                            else Api.dbDeleteUserPg(clientFor(server), name)
                        }
                        confirmDel = null
                    }
                }) { Text("删除", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { confirmDel = null }) { Text("取消") } },
        )
    }
}
