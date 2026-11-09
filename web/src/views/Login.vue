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
          <h2>登录面板</h2>
          <p>使用管理员分配的账号继续</p>
        </div>

        <a-alert v-if="errorMsg" type="error" :message="errorMsg" show-icon class="login-error" />

        <a-form layout="vertical" @finish="submit">
          <a-form-item label="用户名">
            <a-input v-model:value="username" size="large" placeholder="用户名" autocomplete="username" @input="errorMsg = ''">
              <template #prefix><UserOutlined /></template>
            </a-input>
          </a-form-item>
          <a-form-item label="密码">
            <a-input-password v-model:value="password" size="large" placeholder="密码" autocomplete="current-password" @input="errorMsg = ''" @press-enter="submit">
              <template #prefix><LockOutlined /></template>
            </a-input-password>
          </a-form-item>
          <a-form-item v-if="need2fa" label="两因素验证码">
            <a-input v-model:value="totpCode" size="large" placeholder="6 位动态码" inputmode="numeric" maxlength="6" @input="errorMsg = ''" @press-enter="submit">
              <template #prefix><SafetyOutlined /></template>
            </a-input>
          </a-form-item>
          <a-button type="primary" size="large" html-type="submit" block :loading="loading" class="login-submit">登录</a-button>
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
    }
  } finally {
    loading.value = false
  }
}
</script>
