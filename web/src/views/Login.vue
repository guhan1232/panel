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

        <div class="login-panel__foot"><strong>安全入口保护已启用</strong><br />连续登录失败会触发锁定，全部写操作可在审计日志中查证。</div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { UserOutlined, LockOutlined, CheckCircleFilled, SafetyOutlined } from '@ant-design/icons-vue'
import { useAuth } from '../stores/auth'
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
</script>
