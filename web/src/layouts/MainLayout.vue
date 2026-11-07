<template>
  <el-container style="height: 100vh">
    <el-aside :width="collapsed ? '64px' : '216px'" class="aside">
      <div class="logo" :class="{ mini: collapsed }">
        <span class="logo-mark">⬢</span>
        <transition name="fade">
          <span v-show="!collapsed" class="logo-text">webmblx</span>
        </transition>
      </div>
      <el-scrollbar class="menu-scroll">
        <el-menu :collapse="collapsed" :collapse-transition="false" :default-active="$route.path" router
          background-color="transparent" :text-color="'var(--wm-sidebar-text)'" active-text-color="#ffffff"
          class="side-menu">
          <el-menu-item index="/dashboard">
            <el-icon><Odometer /></el-icon><template #title>仪表盘</template>
          </el-menu-item>

          <template v-if="!collapsed"><div class="menu-group">站点与服务</div></template>
          <div v-else class="menu-divider"></div>
          <el-menu-item index="/website"><el-icon><Connection /></el-icon><template #title>网站</template></el-menu-item>
          <el-menu-item index="/databases"><el-icon><Coin /></el-icon><template #title>数据库</template></el-menu-item>
          <el-menu-item index="/appstore"><el-icon><Shop /></el-icon><template #title>应用商店</template></el-menu-item>
          <el-menu-item index="/terminal"><el-icon><Monitor /></el-icon><template #title>终端</template></el-menu-item>

          <template v-if="!collapsed"><div class="menu-group">系统运维</div></template>
          <div v-else class="menu-divider"></div>
          <el-menu-item index="/cron"><el-icon><Timer /></el-icon><template #title>计划任务</template></el-menu-item>
          <el-menu-item index="/alerts"><el-icon><Warning /></el-icon><template #title>告警策略</template></el-menu-item>
          <el-menu-item index="/files"><el-icon><Folder /></el-icon><template #title>文件管理</template></el-menu-item>
          <el-menu-item index="/proc"><el-icon><Cpu /></el-icon><template #title>进程管理</template></el-menu-item>
          <el-menu-item index="/firewall"><el-icon><Lock /></el-icon><template #title>防火墙</template></el-menu-item>
          <el-menu-item index="/certs"><el-icon><Stamp /></el-icon><template #title>SSL 证书</template></el-menu-item>

          <template v-if="!collapsed"><div class="menu-group">面板</div></template>
          <div v-else class="menu-divider"></div>
          <el-menu-item index="/audit"><el-icon><Document /></el-icon><template #title>审计日志</template></el-menu-item>
          <el-menu-item index="/notify"><el-icon><Bell /></el-icon><template #title>通知与存储</template></el-menu-item>
          <el-menu-item index="/setting"><el-icon><Setting /></el-icon><template #title>系统设置</template></el-menu-item>
        </el-menu>
      </el-scrollbar>
      <transition name="fade">
        <div v-show="!collapsed" class="aside-foot">
          <el-icon><User /></el-icon>
          <span>{{ auth.username }}</span>
        </div>
      </transition>
    </el-aside>

    <el-container class="right">
      <el-header class="header" height="56px">
        <div class="header-left">
          <el-icon class="hbtn" @click="collapsed = !collapsed">
            <Expand v-if="collapsed" /><Fold v-else />
          </el-icon>
          <el-breadcrumb separator="/">
            <el-breadcrumb-item>webmblx</el-breadcrumb-item>
            <el-breadcrumb-item>{{ $route.meta.title }}</el-breadcrumb-item>
          </el-breadcrumb>
        </div>
        <div class="header-right">
          <el-tooltip :content="theme.dark ? '切换亮色' : '切换暗色'" placement="bottom">
            <el-icon class="hbtn" @click="theme.toggle()">
              <Sunny v-if="theme.dark" /><Moon v-else />
            </el-icon>
          </el-tooltip>
          <el-dropdown @command="onCommand">
            <span class="user">
              <el-avatar :size="26" class="user-avatar">{{ (auth.username || 'U').slice(0, 1).toUpperCase() }}</el-avatar>
              <span class="user-name">{{ auth.username }}</span>
              <el-icon><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item command="logout">
                  <el-icon><SwitchButton /></el-icon>退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>
      <el-main class="main"><router-view /></el-main>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  Odometer, Connection, Coin, Shop, Monitor, Timer, Warning, Folder, Cpu,
  Lock, Stamp, Document, Bell, Setting, User, Expand, Fold, Sunny, Moon,
  ArrowDown, SwitchButton
} from '@element-plus/icons-vue'
import { useAuth } from '../stores/auth'
import { useTheme } from '../stores/theme'

const auth = useAuth()
const theme = useTheme()
const router = useRouter()
const collapsed = ref(false)

async function onCommand(cmd: string) {
  if (cmd === 'logout') {
    await auth.logout()
    router.push('/login')
  }
}
</script>

<style scoped>
.aside {
  background: var(--wm-sidebar-bg);
  display: flex;
  flex-direction: column;
  transition: width 0.2s ease;
  overflow: hidden;
}
.logo {
  height: 56px;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 16px;
  flex-shrink: 0;
}
.logo.mini { justify-content: center; padding: 0; }
.logo-mark {
  width: 32px; height: 32px; border-radius: 9px; flex-shrink: 0;
  background: linear-gradient(135deg, #5ea4ff, #3d7fff);
  color: #fff; font-size: 16px; font-weight: 800;
  display: flex; align-items: center; justify-content: center;
  box-shadow: 0 6px 16px -4px rgba(61, 127, 255, 0.6);
}
.logo-text { color: #fff; font-size: 17px; font-weight: 700; letter-spacing: 1px; }
.menu-scroll { flex: 1; }
.side-menu { border-right: none; padding: 4px 8px; }
.side-menu .el-menu-item {
  height: 42px;
  border-radius: 8px;
  margin: 2px 0;
  transition: background 0.15s ease, color 0.15s ease;
}
.side-menu .el-menu-item:hover { background: rgba(255, 255, 255, 0.06); }
.side-menu .el-menu-item.is-active {
  background: var(--wm-sidebar-active-bg);
  color: #fff;
  font-weight: 600;
}
.menu-group {
  color: rgba(255, 255, 255, 0.32);
  font-size: 11px;
  letter-spacing: 2px;
  padding: 14px 12px 6px;
  user-select: none;
}
.menu-divider { height: 1px; margin: 10px 14px; background: rgba(255, 255, 255, 0.08); }
.aside-foot {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--wm-sidebar-text);
  font-size: 13px;
  padding: 12px 18px;
  border-top: 1px solid rgba(255, 255, 255, 0.07);
  flex-shrink: 0;
}

.right { min-width: 0; }
.header {
  background: var(--wm-header-bg);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  border-bottom: 1px solid var(--wm-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  position: relative;
  z-index: 10;
}
.header-left, .header-right { display: flex; align-items: center; gap: 14px; }
.hbtn { font-size: 18px; cursor: pointer; color: var(--wm-text-2); transition: color 0.15s; }
.hbtn:hover { color: var(--el-color-primary); }
.user { cursor: pointer; display: inline-flex; align-items: center; gap: 7px; color: var(--wm-text); }
.user-avatar { background: linear-gradient(135deg, #5ea4ff, #3d7fff); font-size: 13px; font-weight: 700; }
.user-name { font-size: 13px; }

.main { padding: 20px; overflow: auto; }

.fade-enter-active, .fade-leave-active { transition: opacity 0.18s ease; }
.fade-enter-from, .fade-leave-to { opacity: 0; }
</style>
