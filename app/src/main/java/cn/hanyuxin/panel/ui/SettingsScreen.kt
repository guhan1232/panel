// SettingsScreen.kt —— 1.3.0 系统设置：面板名称 / 修改管理员密码 / 开放 API Token。
package cn.hanyuxin.panel.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ArrowBack
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TopAppBar
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
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import cn.hanyuxin.panel.data.Api
import cn.hanyuxin.panel.data.ServerInfo
import cn.hanyuxin.panel.data.clientFor
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(server: ServerInfo) {
    val scope = rememberCoroutineScope()
    val client = clientFor(server)
    val snackbarHost = remember { mutableStateOf<String?>(null) }

    // 面板名称
    var panelName by remember { mutableStateOf("") }
    var savingName by remember { mutableStateOf(false) }

    // 修改密码
    var oldPwd by remember { mutableStateOf("") }
    var newPwd by remember { mutableStateOf("") }
    var confirmPwd by remember { mutableStateOf("") }
    var changingPwd by remember { mutableStateOf(false) }

    // API Token
    var tokenEnabled by remember { mutableStateOf(false) }
    var tokenValue by remember { mutableStateOf("") }
    var tokenShown by remember { mutableStateOf(false) }
    var tokenBusy by remember { mutableStateOf(false) }

    fun toast(s: String?) { snackbarHost.value = s ?: "操作失败，请重试" }

    LaunchedEffect(Unit) {
        apiGuard(onError = { toast(it.message) }) {
            val b = Api.brandInfo(client)
            panelName = b.name.trim()
        }
        apiGuard(onError = { toast(it.message) }) {
            val (en, tk) = Api.mcpTokenStatus(client)
            tokenEnabled = en
            tokenValue = tk
        }
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = { Text("系统设置", fontWeight = FontWeight.Bold) },
                navigationIcon = {
                    IconButton(onClick = { Router.pop() }) { Icon(Icons.Outlined.ArrowBack, "返回") }
                },
            )
        },
    ) { pad ->
        Column(
            Modifier.padding(pad).fillMaxSize().verticalScroll(rememberScrollState()).padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            snackbarHost.value?.let {
                Text(it, color = MaterialTheme.colorScheme.primary, fontSize = 12.sp)
            }

            // ---- 面板名称 ----
            PanelCard {
                Text("面板名称", fontWeight = FontWeight.SemiBold)
                Text(
                    "自定义侧栏与登录页显示的名称（1-40 字）；留空恢复默认「寒语馨面板」",
                    fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(panelName, { panelName = it }, singleLine = true, modifier = Modifier.fillMaxWidth())
                Spacer(Modifier.height(8.dp))
                Button(
                    enabled = !savingName,
                    onClick = {
                        scope.launch {
                            savingName = true
                            apiGuard(onError = { toast(it.message) }) {
                                val d = Api.panelNameSet(client, panelName.trim())
                                panelName = (d?.name ?: "").trim()
                                toast("面板名称已保存")
                            }
                            savingName = false
                        }
                    },
                ) { Text("保存名称") }
            }
        
            // ---- 修改管理员密码 ----
            PanelCard {
                Text("修改管理员密码", fontWeight = FontWeight.SemiBold)
                Text(
                    "新密码至少 8 位，修改后当前会话保持有效",
                    fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(8.dp))
                OutlinedTextField(oldPwd, { oldPwd = it }, label = { Text("原密码") }, singleLine = true, visualTransformation = PasswordVisualTransformation(), modifier = Modifier.fillMaxWidth())
                Spacer(Modifier.height(6.dp))
                OutlinedTextField(newPwd, { newPwd = it }, label = { Text("新密码") }, singleLine = true, visualTransformation = PasswordVisualTransformation(), modifier = Modifier.fillMaxWidth())
                Spacer(Modifier.height(6.dp))
                OutlinedTextField(confirmPwd, { confirmPwd = it }, label = { Text("确认密码") }, singleLine = true, visualTransformation = PasswordVisualTransformation(), modifier = Modifier.fillMaxWidth())
                Spacer(Modifier.height(8.dp))
                Button(
                    enabled = !changingPwd && oldPwd.isNotBlank() && newPwd.isNotBlank() && newPwd == confirmPwd,
                    onClick = {
                        scope.launch {
                            changingPwd = true
                            var ok = false
                            apiGuard(onError = { toast(it.message) }) { Api.changePassword(client, oldPwd, newPwd); ok = true }
                            if (ok) {
                                oldPwd = ""; newPwd = ""; confirmPwd = ""
                                toast("密码已修改")
                            }
                            changingPwd = false
                        }
                    },
                ) { Text("修改密码") }
            }

            // ---- 开放 API Token ----
            PanelCard {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Text("开放 API Token", fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
                    if (tokenEnabled) StatusPill("已启用", OkGreen) else StatusPill("未启用", IdleGray)
                }
                Text(
                    "供外部脚本直调面板 API（等效管理员权限，仅经 HTTPS 调用）",
                    fontSize = 11.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Spacer(Modifier.height(8.dp))
                if (tokenEnabled && tokenValue.isNotBlank()) {
                    Text(
                        if (tokenShown) tokenValue else tokenValue.take(8) + "••••••••",
                        fontSize = 12.sp, fontFamily = FontFamily.Monospace,
                    )
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        TextButton(onClick = { tokenShown = !tokenShown }) { Text(if (tokenShown) "隐藏" else "显示") }
                        TextButton(onClick = { toast("重置后客户端需更新 Token") }) { Text("重置后立即失效") }
                    }
                }
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    if (tokenBusy) CircularProgressIndicator(Modifier.width(22.dp).height(22.dp))
                    Button(enabled = !tokenBusy, onClick = {
                        scope.launch {
                            tokenBusy = true
                            apiGuard(onError = { toast(it.message) }) {
                                val d = Api.mcpTokenGenerate(client)
                                tokenValue = d
                                tokenEnabled = true
                                tokenShown = true
                                toast("Token 已生成")
                            }
                            tokenBusy = false
                        }
                    }) { Text(if (tokenEnabled) "重置 Token" else "生成 Token 启用") }
                    if (tokenEnabled) {
                        TextButton(enabled = !tokenBusy, onClick = {
                            scope.launch {
                                tokenBusy = true
                                var ok = false
                                apiGuard(onError = { toast(it.message) }) { Api.mcpTokenDisable(client); ok = true }
                                if (ok) {
                                    tokenEnabled = false; tokenValue = ""; tokenShown = false
                                    toast("已禁用")
                                }
                                tokenBusy = false
                            }
                        }) { Text("禁用", color = MaterialTheme.colorScheme.error) }
                    }
                }
            }
        }
    }
}
