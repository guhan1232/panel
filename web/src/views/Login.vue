<template>
  <div class="login-bg">
    <el-card class="login-card">
      <h2 style="text-align: center; margin: 8px 0 24px">⬢ webmblx 面板</h2>
      <el-form @submit.prevent="submit">
        <el-form-item>
          <el-input v-model="username" placeholder="用户名" size="large" :prefix-icon="User" />
        </el-form-item>
        <el-form-item>
          <el-input v-model="password" type="password" placeholder="密码" size="large" :prefix-icon="Lock" show-password @keyup.enter="submit" />
        </el-form-item>
        <el-button type="primary" size="large" style="width: 100%" :loading="loading" native-type="submit">登 录</el-button>
      </el-form>
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
.login-bg {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #1d2530 0%, #2b3a4e 60%, #35506b 100%);
}
.login-card { width: 380px; border-radius: 10px; }
</style>
