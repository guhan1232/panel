// MainActivity.kt —— 单 Activity 入口 + 生物识别锁门（docs/APP.md §4.3）。
package cn.hanyuxin.panel

import android.content.Intent
import android.os.Bundle
import androidx.activity.compose.setContent
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricPrompt
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Fingerprint
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity
import cn.hanyuxin.panel.data.QrData
import cn.hanyuxin.panel.data.ServerStore
import cn.hanyuxin.panel.ui.handleScan
import cn.hanyuxin.panel.ui.AppTheme
import cn.hanyuxin.panel.ui.RootApp

class MainActivity : FragmentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        ServerStore.init(this)
        handleDeepLink(intent)
        setContent {
            AppTheme {
                LockGate {
                    RootApp()
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleDeepLink(intent)
    }

    /** hanyuxin://login|bind 深链：与扫码同构分发（Web 一键唤起绑定）。 */
    private fun handleDeepLink(intent: Intent?) {
        val data = intent?.data?.toString() ?: return
        val qr = QrData.parse(data) ?: return
        runOnUiThread { handleScan(qr) }
    }
}

/** 生物识别门：设置页开启后，每次冷启动需指纹/锁屏凭据解锁。 */
@Composable
private fun LockGate(content: @Composable () -> Unit) {
    val ctx = LocalContext.current
    var unlocked by remember { mutableStateOf(!ServerStore.bioEnabled()) }
    var attempt by remember { mutableIntStateOf(0) }

    fun prompt() {
        val activity = ctx as? FragmentActivity ?: return
        val pm = BiometricPrompt(activity, ContextCompat.getMainExecutor(activity), object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                unlocked = true
            }
        })
        pm.authenticate(
            BiometricPrompt.PromptInfo.Builder()
                .setTitle("解锁寒语馨面板")
                .setSubtitle("验证指纹或锁屏凭据以继续")
                .setAllowedAuthenticators(
                    BiometricManager.Authenticators.BIOMETRIC_WEAK or BiometricManager.Authenticators.DEVICE_CREDENTIAL,
                )
                .build(),
        )
    }

    LaunchedEffect(attempt) {
        if (ServerStore.bioEnabled() && !unlocked && attempt > 0) prompt()
    }
    LaunchedEffect(Unit) {
        if (ServerStore.bioEnabled() && !unlocked) attempt++
    }

    if (unlocked) {
        content()
    } else {
        Column(
            Modifier.fillMaxSize(),
            verticalArrangement = Arrangement.Center,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Icon(Icons.Outlined.Fingerprint, null, Modifier.size(64.dp), tint = MaterialTheme.colorScheme.primary)
            Spacer(Modifier.height(12.dp))
            Text("已锁定", style = MaterialTheme.typography.titleMedium)
            Spacer(Modifier.height(16.dp))
            Button(onClick = { attempt++ }) { Text("解锁") }
        }
    }
}
