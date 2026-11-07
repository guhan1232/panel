<template>
  <div class="login-bg">
    <div class="blob b1"></div>
    <div class="blob b2"></div>
    <el-card class="login-card" shadow="never">
      <div class="login-brand">
        <span class="logo-mark">⬢</span>
        <h2>webmblx</h2>
      </div>
      <div class="login-tagline">Linux 服务器管理面板</div>
      <el-form @submit.prevent="submit">
        <el-form-item>
          <el-input v-model="username" placeholder="用户名" size="large" :prefix-icon="User" autocomplete="username" />
        </el-form-item>
        <el-form-item>
          <el-input v-model="password" type="password" placeholder="密码" size="large" :prefix-icon="Lock" show-password autocomplete="current-password" @keyup.enter="submit" />
        </el-form-item>
        <el-button type="primary" size="large" style="width: 100%; letter-spacing: 6px" :loading="loading" native-type="submit">登 录</el-button>
      </el-form>
      <div class="login-foot">安全入口保护已启用 · 会话与操作全程审计</div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { User, Lock } from '@element-plus/icons-vue'
import { useAuth } from '../stores/auth'

const auth = useAuth()
const router = useRouter()
const username = ref('')
const password = ref('')
const loading = ref(false)

async function submit() {
  if (!username.value || !password.value) {
    ElMessage.warning('请输入用户名和密码')
    return
  }
  loading.value = true
  try {
    await auth.login(username.value, password.value)
    router.push('/')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-foot {
  margin-top: 18px;
  text-align: center;
  color: rgba(255, 255, 255, 0.35);
  font-size: 12px;
}
</style>
