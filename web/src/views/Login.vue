<template>
  <div class="login-bg">
    <div class="login-shell">
      <!-- Left: product context, no fabricated metrics -->
      <aside class="login-aside">
        <div>
          <div class="login-aside__brand">
            <span class="wm-brand__mark" aria-hidden="true">W</span>
            <div>
              <div class="login-aside__name">webmblx</div>
              <div class="login-aside__tag">Linux server console</div>
            </div>
          </div>
          <h1 class="login-aside__headline">一台服务器，<br />一个清晰的操作面。</h1>
          <p class="login-aside__lead">站点、任务、证书、进程与节点集中管理；每一次写操作都留有审计记录。</p>
          <div class="login-aside__facts">
            <div class="login-fact"><span class="login-fact__i"><el-icon><Lock /></el-icon></span>安全入口 + 会话鉴权</div>
            <div class="login-fact"><span class="login-fact__i"><el-icon><Document /></el-icon></span>全量写操作审计</div>
            <div class="login-fact"><span class="login-fact__i"><el-icon><Connection /></el-icon></span>本机与远程节点统一视图</div>
          </div>
        </div>
        <div class="login-aside__foot">webmblx / control surface</div>
      </aside>

      <!-- Right: authentication -->
      <section class="login-panel">
        <div class="login-panel__eyebrow">secure access</div>
        <h2 class="login-panel__title">登录面板</h2>
        <p class="login-panel__desc">请使用管理员分配的账号继续</p>

        <WmNotice v-if="errorMsg" tone="danger" class="login-error">{{ errorMsg }}</WmNotice>

        <el-form @submit.prevent="submit">
          <el-form-item>
            <el-input
              v-model="username"
              placeholder="用户名"
              size="large"
              :prefix-icon="User"
              autocomplete="username"
              @input="errorMsg = ''"
            />
          </el-form-item>
          <el-form-item>
            <el-input
              v-model="password"
              type="password"
              placeholder="密码"
              size="large"
              :prefix-icon="Lock"
              show-password
              autocomplete="current-password"
              @input="errorMsg = ''"
              @keyup.enter="submit"
            />
          </el-form-item>
          <el-button
            type="primary"
            size="large"
            class="login-submit"
            :loading="loading"
            native-type="submit"
          >登 录</el-button>
        </el-form>

        <div class="login-panel__foot">
          <strong>安全入口保护已启用</strong><br />
          会话与全部写操作均被记录，连续失败将触发登录锁定。
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { User, Lock, Document, Connection } from '@element-plus/icons-vue'
import { useAuth } from '../stores/auth'

const auth = useAuth()
const router = useRouter()
const username = ref('')
const password = ref('')
const loading = ref(false)
const errorMsg = ref('')

async function submit() {
  if (!username.value || !password.value) {
    errorMsg.value = '请输入用户名和密码后重试'
    return
  }
  errorMsg.value = ''
  loading.value = true
  try {
    await auth.login(username.value, password.value)
    router.push('/')
  } finally {
    loading.value = false
  }
}
</script>
