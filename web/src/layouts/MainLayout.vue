<template>
  <div
    class="wm-shell"
    :class="{ 'wm-shell--mini': collapsed && !isMobile, 'wm-shell--drawer': drawerOpen }"
  >
    <!-- Navigation rail ------------------------------------------------ -->
    <aside class="wm-rail" aria-label="主导航">
      <div class="wm-brand">
        <span class="wm-brand__mark" aria-hidden="true">W</span>
        <div class="wm-brand__text">
          <div class="wm-brand__name">webmblx</div>
          <div class="wm-brand__sub">control surface</div>
        </div>
      </div>

      <nav class="wm-rail__nav">
        <div v-for="(group, gi) in navGroups" :key="group.label" class="wm-nav-group">
          <template v-if="!collapsed || isMobile">
            <div class="wm-nav-group__label">
              <span class="wm-mono" style="opacity: .65; margin-right: 8px">{{ String(gi + 1).padStart(2, '0') }}</span>
              {{ group.label }}
            </div>
          </template>
          <div v-else class="wm-nav-group__rule" />

          <router-link
            v-for="item in group.items"
            :key="item.path"
            :to="item.path"
            class="wm-nav-item"
            :class="{ 'is-active': route.path === item.path }"
            :title="collapsed && !isMobile ? item.label : undefined"
            :aria-current="route.path === item.path ? 'page' : undefined"
            @click="drawerOpen = false"
          >
            <el-icon><component :is="item.icon" /></el-icon>
            <span class="wm-nav-item__label">{{ item.label }}</span>
          </router-link>
        </div>
      </nav>

      <div class="wm-rail__foot">
        <span class="wm-rail__avatar">{{ (auth.username || 'U').slice(0, 1).toUpperCase() }}</span>
        <div class="wm-rail__who">
          <div class="wm-rail__name wm-truncate">{{ auth.username || '未登录' }}</div>
          <div class="wm-rail__role">{{ roleLabel }}</div>
        </div>
      </div>
    </aside>

    <div class="wm-scrim" @click="drawerOpen = false" />

    <!-- Main column ----------------------------------------------------- -->
    <div class="wm-main">
      <header class="wm-topbar">
        <div class="wm-topbar__left">
          <button
            class="wm-icon-btn"
            type="button"
            :aria-label="isMobile ? '打开导航' : collapsed ? '展开导航' : '收起导航'"
            :aria-expanded="isMobile ? drawerOpen : !collapsed"
            @click="toggleNav"
          >
            <el-icon><Menu v-if="isMobile" /><Expand v-else-if="collapsed" /><Fold v-else /></el-icon>
          </button>
          <div class="wm-crumb" aria-label="当前位置">
            <span>{{ route.meta.group || '面板' }}</span>
            <span class="wm-crumb__sep">/</span>
            <span class="wm-crumb__current">{{ route.meta.title }}</span>
          </div>
          <span class="wm-topbar__title">{{ route.meta.title }}</span>
        </div>

        <div class="wm-topbar__right">
          <span class="wm-session" :title="secure ? '当前通过 HTTPS 访问' : '当前为 HTTP 明文连接'">
            <i class="wm-session__dot" :style="secure ? '' : 'background: var(--wm-warning); box-shadow: 0 0 0 3px rgba(185,116,27,.16)'" />
            {{ secure ? 'HTTPS 安全会话' : 'HTTP 明文会话' }}
          </span>
          <div class="wm-topbar__divider" />
          <el-tooltip :content="theme.dark ? '切换到亮色' : '切换到暗色'" placement="bottom">
            <button class="wm-icon-btn" type="button" :aria-label="theme.dark ? '切换到亮色' : '切换到暗色'" @click="theme.toggle()">
              <el-icon><Sunny v-if="theme.dark" /><Moon v-else /></el-icon>
            </button>
          </el-tooltip>
          <el-dropdown trigger="click" @command="onCommand">
            <button class="wm-user-trigger" type="button" aria-label="用户菜单">
              <el-avatar :size="26" class="wm-user-avatar">{{ (auth.username || 'U').slice(0, 1).toUpperCase() }}</el-avatar>
              <span class="wm-user-trigger__name">{{ auth.username }}</span>
              <el-icon :size="12"><ArrowDown /></el-icon>
            </button>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item disabled>
                  <span class="wm-eyebrow">当前角色 · {{ roleLabel }}</span>
                </el-dropdown-item>
                <el-dropdown-item command="theme" divided>
                  <el-icon><Moon v-if="!theme.dark" /><Sunny v-else /></el-icon>
                  {{ theme.dark ? '亮色外观' : '暗色外观' }}
                </el-dropdown-item>
                <el-dropdown-item command="logout" divided>
                  <el-icon><SwitchButton /></el-icon>退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>

      <main class="wm-content">
        <div class="wm-content__inner">
          <WmPageHeader
            :eyebrow="String(route.meta.group || '控制台')"
            :title="String(route.meta.title || '')"
            :description="String(route.meta.desc || '')"
          >
            <template #meta>
              <WmStatusPill :label="roleLabel" tone="info" :dot="false" />
            </template>
          </WmPageHeader>
          <router-view v-slot="{ Component }">
            <component :is="Component" />
          </router-view>
        </div>
      </main>
    </div>

    <!-- Mobile bottom navigation ---------------------------------------- -->
    <nav class="wm-mobilebar" aria-label="快捷导航">
      <router-link
        v-for="item in mobileItems"
        :key="item.path"
        :to="item.path"
        class="wm-mobilebar__item"
        :class="{ 'is-active': route.path === item.path }"
      >
        <el-icon><component :is="item.icon" /></el-icon>
        <span>{{ item.label }}</span>
      </router-link>
      <button class="wm-mobilebar__item" type="button" @click="drawerOpen = true">
        <el-icon><MoreFilled /></el-icon>
        <span>更多</span>
      </button>
    </nav>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Odometer, Connection, Coin, Shop, Monitor, Timer, Warning, Folder, Cpu,
  Lock, Stamp, Document, Bell, Setting, Expand, Fold, Sunny, Moon,
  ArrowDown, SwitchButton, Avatar, Share, Menu, MoreFilled, Box
} from '@element-plus/icons-vue'
import { useAuth } from '../stores/auth'
import { useTheme } from '../stores/theme'

const auth = useAuth()
const theme = useTheme()
const route = useRoute()
const router = useRouter()

const collapsed = ref(localStorage.getItem('wm_rail_mini') === '1')
const drawerOpen = ref(false)
const isMobile = ref(false)
const secure = location.protocol === 'https:'

const ROLE_LABEL: Record<string, string> = { admin: '管理员', operator: '操作员', viewer: '只读' }
const roleLabel = computed(() => ROLE_LABEL[auth.role] || '未登录')

const navGroups = computed(() => [
  {
    label: '概览',
    items: [{ path: '/dashboard', label: '仪表盘', icon: Odometer }]
  },
  {
    label: '站点与服务',
    items: [
      { path: '/website', label: '网站', icon: Connection },
      { path: '/databases', label: '数据库', icon: Coin },
      { path: '/softstore', label: '软件商店', icon: Shop },
      { path: '/dockerstore', label: 'Docker 容器管理', icon: Box },
      ...(auth.role !== 'viewer' ? [{ path: '/terminal', label: '终端', icon: Monitor }] : [])
    ]
  },
  {
    label: '系统运维',
    items: [
      { path: '/cron', label: '计划任务', icon: Timer },
      { path: '/alerts', label: '告警策略', icon: Warning },
      { path: '/files', label: '文件管理', icon: Folder },
      { path: '/proc', label: '进程管理', icon: Cpu },
      { path: '/nodes', label: '节点管理', icon: Share },
      { path: '/firewall', label: '防火墙', icon: Lock },
      { path: '/certs', label: 'SSL 证书', icon: Stamp }
    ]
  },
  {
    label: '面板',
    items: [
      { path: '/audit', label: '审计日志', icon: Document },
      { path: '/notify', label: '通知与存储', icon: Bell },
      ...(auth.role === 'admin' ? [{ path: '/users', label: '用户管理', icon: Avatar }] : []),
      { path: '/setting', label: '系统设置', icon: Setting }
    ]
  }
])

const mobileItems = [
  { path: '/dashboard', label: '仪表盘', icon: Odometer },
  { path: '/website', label: '网站', icon: Connection },
  { path: '/cron', label: '计划任务', icon: Timer },
  { path: '/files', label: '文件', icon: Folder }
]

function toggleNav() {
  if (isMobile.value) drawerOpen.value = !drawerOpen.value
  else {
    collapsed.value = !collapsed.value
    localStorage.setItem('wm_rail_mini', collapsed.value ? '1' : '0')
  }
}

async function onCommand(cmd: string) {
  if (cmd === 'theme') theme.toggle()
  if (cmd === 'logout') {
    await auth.logout()
    router.push('/login')
  }
}

let mql: MediaQueryList | null = null
function onViewport(e: MediaQueryListEvent | MediaQueryList) {
  isMobile.value = e.matches
  if (!e.matches) drawerOpen.value = false
}

watch(() => route.fullPath, () => { drawerOpen.value = false })

onMounted(() => {
  mql = window.matchMedia('(max-width: 900px)')
  onViewport(mql)
  mql.addEventListener('change', onViewport)
})
onBeforeUnmount(() => mql?.removeEventListener('change', onViewport))
</script>

<style scoped>
.wm-user-avatar {
  background: var(--wm-accent-soft);
  color: var(--wm-accent);
  font-weight: 700;
  font-size: 12px;
  border: 1px solid var(--wm-accent-line);
}
.wm-topbar__title { display: none; font-size: 14px; font-weight: 650; color: var(--wm-ink); }
@media (max-width: 900px) {
  .wm-topbar__title { display: block; }
}
</style>
