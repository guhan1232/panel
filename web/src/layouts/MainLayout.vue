<template>
  <el-container style="height: 100vh">
    <el-aside width="200px" class="aside">
      <div class="logo">⬢ webmblx</div>
      <el-menu :default-active="$route.path" router background-color="#1d2530" text-color="#aeb9c9" active-text-color="#409eff" style="border-right: none">
        <el-menu-item index="/dashboard"><el-icon><Odometer /></el-icon>仪表盘</el-menu-item>
        <el-menu-item index="/website"><el-icon><Connection /></el-icon>网站</el-menu-item>
        <el-menu-item index="/databases"><el-icon><Coin /></el-icon>数据库</el-menu-item>
        <el-menu-item index="/firewall"><el-icon><Lock /></el-icon>防火墙</el-menu-item>
        <el-menu-item index="/certs"><el-icon><Stamp /></el-icon>SSL 证书</el-menu-item>
        <el-menu-item index="/cron"><el-icon><Timer /></el-icon>计划任务</el-menu-item>
        <el-menu-item index="/files"><el-icon><Folder /></el-icon>文件管理</el-menu-item>
        <el-menu-item index="/terminal"><el-icon><Monitor /></el-icon>终端</el-menu-item>
        <el-menu-item index="/proc"><el-icon><Cpu /></el-icon>进程管理</el-menu-item>
        <el-menu-item index="/audit"><el-icon><Document /></el-icon>审计日志</el-menu-item>
        <el-menu-item index="/notify"><el-icon><Bell /></el-icon>通知与存储</el-menu-item>
        <el-menu-item index="/setting"><el-icon><Setting /></el-icon>系统设置</el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header class="header">
        <span class="title">{{ $route.meta.title || 'webmblx' }}</span>
        <el-dropdown @command="onCommand">
          <span class="user"><el-icon><User /></el-icon>&nbsp;{{ auth.username }}</span>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="logout">退出登录</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </el-header>
      <el-main style="padding: 16px; overflow: auto">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { Odometer, Timer, Folder, Monitor, Cpu, Setting, User, Connection, Coin, Lock, Stamp, Document, Bell } from '@element-plus/icons-vue'
import { useAuth } from '../stores/auth'

const auth = useAuth()
const router = useRouter()

async function onCommand(cmd: string) {
  if (cmd === 'logout') {
    await auth.logout()
    router.push('/login')
  }
}
</script>

<style scoped>
.aside { background: #1d2530; }
.logo {
  color: #fff;
  font-size: 18px;
  font-weight: 700;
  padding: 18px 20px;
  letter-spacing: 1px;
}
.header {
  background: #fff;
  display: flex;
  align-items: center;
  justify-content: space-between;
  box-shadow: 0 1px 4px rgba(0, 21, 41, 0.08);
}
.title { font-size: 16px; font-weight: 600; }
.user { cursor: pointer; display: inline-flex; align-items: center; color: #333; }
</style>
