<template>
  <div class="login-bg">
    <div class="login-shell">
      <div class="login-aside">
        <div>
          <div class="login-brand"><span class="login-brand__mark">寒</span><span>寒语馨面板</span></div>
          <h1>服务器管理，<br />从一个清晰的入口开始。</h1>
          <p>站点、任务、证书、进程与节点集中管理。观察、操作、留痕，都在同一处完成。</p>
          <div class="login-facts">
            <div><CheckCircleFilled />安全入口保护</div>
            <div><CheckCircleFilled />会话鉴权与登录锁定</div>
            <div><CheckCircleFilled />全量写操作审计</div>
          </div>
        </div>
        <div class="login-aside__foot">寒语馨面板 / Hanyuxin Panel</div>
      </div>

      <section class="login-panel">
        <div class="login-panel__head">
          <h2>{{ tr('登录面板') }}</h2>
          <p>{{ tr('使用管理员分配的账号继续') }}</p>
        </div>

        <a-alert v-if="errorMsg" type="error" :message="errorMsg" show-icon class="login-error" />

        <a-tabs v-model:activeKey="tab" centered>
          <a-tab-pane key="pwd" :tab="tr('账密登录')">
            <a-form layout="vertical" @submit.prevent="submit">
              <a-form-item :label="tr('用户名')">
                <a-input v-model:value="username" size="large" :placeholder="tr('用户名')" autocomplete="username" @input="errorMsg = ''">
                  <template #prefix><UserOutlined /></template>
                </a-input>
              </a-form-item>
              <a-form-item :label="tr('密码')">
                <a-input-password v-model:value="password" size="large" :placeholder="tr('密码')" autocomplete="current-password" @input="errorMsg = ''">
                  <template #prefix><LockOutlined /></template>
                </a-input-password>
              </a-form-item>
              <a-form-item v-if="need2fa" :label="tr('两因素验证码')">
                <a-input v-model:value="totpCode" size="large" :placeholder="tr('6 位动态码')" inputmode="numeric" maxlength="6" @input="errorMsg = ''">
                  <template #prefix><SafetyOutlined /></template>
                </a-input>
              </a-form-item>
              <a-button type="primary" size="large" html-type="submit" block :loading="loading" class="login-submit">{{ tr('登录') }}</a-button>
            </a-form>
          </a-tab-pane>
          <a-tab-pane key="qr" :tab="tr('扫码登录')">
            <div class="qr-wrap">
              <template v-if="qrState !== 'expired'">
                <div class="qr-box" :class="{ 'qr-box--scanned': qrState === 'scanned' }">
                  <QrcodeVue v-if="qrText" :value="qrText" :size="188" level="M" render-as="svg" />
                  <div v-if="qrState === 'scanned'" class="qr-scan-mask">
                    <MobileOutlined class="qr-scan-mask__icon" />
                    <span>已扫码 · 请在手机上确认</span>
                  </div>
                </div>
                <p class="qr-tip">
                  <template v-if="qrState === 'pending'">打开寒语馨面板 APP，使用「扫一扫」扫码登录</template>
                  <template v-else-if="qrState === 'scanned'">扫码成功，等待手机端确认登录…</template>
                  <template v-else>正在生成二维码…</template>
                </p>
              </template>
              <template v-else>
                <div class="qr-expired">
                  <p>二维码已过期</p>
                  <a-button type="primary" @click="startQr">刷新二维码</a-button>
                </div>
              </template>
            </div>
          </a-tab-pane>
        </a-tabs>

        <div class="login-panel__foot"><strong>安全入口保护已启用</strong><br />连续登录失败会触发锁定，全部写操作可在审计日志中查证。</div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { UserOutlined, LockOutlined, CheckCircleFilled, SafetyOutlined, MobileOutlined } from '@ant-design/icons-vue'
import QrcodeVue from 'vue-qrcode'
import { useAuth } from '../stores/auth'
import api from '../api'
import { t as tr } from '../i18n'

const auth = useAuth()
const router = useRouter()
const username = ref('')
const password = ref('')
const loading = ref(false)
const errorMsg = ref('')
const need2fa = ref(false)
const totpCode = ref('')

async function submit() {
  if (!username.value || !password.value) {
    errorMsg.value = '请输入用户名和密码后重试'
    return
  }
  if (need2fa.value && !totpCode.value) {
    errorMsg.value = '请输入两因素验证码'
    return
  }
  errorMsg.value = ''
  loading.value = true
  try {
    await auth.login(username.value, password.value, totpCode.value)
    router.push('/')
  } catch (e: any) {
    if (e?.need_2fa) {
      need2fa.value = true
      errorMsg.value = '该账号已启用两因素验证，请输入验证码'
    } else {
      // 网络失败 / 密码错误等：给出可见错误，避免静默无响应
      errorMsg.value = e?.msg || e?.response?.data?.msg || (e?.message === 'Network Error' ? '网络错误，请稍后重试' : '') || '登录失败，请检查用户名与密码'
    }
  } finally {
    loading.value = false
  }
}

// ---- 扫码登录（docs/APP.md §5）：create → 展示二维码 → 2s 轮询 → exchange 兑换 Cookie ----
const tab = ref('pwd')
const qrText = ref('')
const qrKey = ref('')
const qrState = ref<'' | 'pending' | 'scanned' | 'confirmed' | 'expired'>('')
let pollTimer: ReturnType<typeof setInterval> | undefined

async function startQr() {
  stopQr()
  qrText.value = ''
  qrState.value = ''
  try {
    const d: any = await api.post('/auth/qr/create', { ua: navigator.userAgent })
    qrKey.value = d.key
    qrText.value = d.qr_text
    qrState.value = 'pending'
    pollTimer = setInterval(poll, 2000)
  } catch {
    qrState.value = 'expired'
  }
}

function stopQr() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = undefined
  }
}

async function poll() {
  if (!qrKey.value) return
  try {
    const d: any = await api.get('/auth/qr/state', { params: { key: qrKey.value } })
    qrState.value = d.state
    if (d.state === 'confirmed' && d.ticket) {
      stopQr()
      await api.post('/auth/qr/exchange', { key: qrKey.value, ticket: d.ticket })
      await auth.fetchMe()
      router.push('/')
    }
  } catch {
    /* 瞬时网络错误：继续轮询 */
  }
}

watch(tab, v => (v === 'qr' ? startQr() : stopQr()))
onUnmounted(stopQr)
</script>

<style scoped>
.qr-wrap {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 8px 0 4px;
}
.qr-box {
  position: relative;
  padding: 12px;
  border: 1px solid var(--wm-border, #e5e7eb);
  border-radius: 12px;
  background: #fff;
  line-height: 0;
}
.qr-box--scanned svg {
  filter: blur(4px) opacity(0.35);
}
.qr-scan-mask {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 6px;
  color: #1677ff;
  font-weight: 600;
}
.qr-scan-mask__icon {
  font-size: 28px;
}
.qr-tip {
  margin: 12px 0 0;
  font-size: 13px;
  color: var(--wm-text-muted, #6b7280);
  text-align: center;
}
.qr-expired {
  padding: 24px 0;
  text-align: center;
  color: var(--wm-text-muted, #6b7280);
}
.qr-expired p {
  margin-bottom: 12px;
}
</style>
