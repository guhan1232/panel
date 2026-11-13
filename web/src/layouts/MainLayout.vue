<template>
  <div class="wm-shell" :class="{ 'wm-shell--mini': collapsed && !isMobile, 'wm-shell--drawer': drawerOpen }">
    <!-- Desktop sider -->
    <aside class="wm-sider">
      <div class="wm-brand">
        <span class="wm-brand__mark">寒</span>
        <transition name="fade">
          <span v-show="!collapsed || isMobile" class="wm-brand__text">寒语馨面板</span>
        </transition>
      </div>
      <div class="wm-sider__scroll">
        <a-menu
          theme="light"
          mode="inline"
          :inline-collapsed="collapsed && !isMobile"
          :selected-keys="[route.path]"
          :open-keys="[]"
          class="wm-nav"
        >
          <a-menu-item-group v-for="group in navGroups" :key="group.label" :title="group.label">
            <a-menu-item
              v-for="item in group.items"
              :key="item.path"
              @click="go(item.path)"
            >
              <template #icon><component :is="item.icon" /></template>
              <span>{{ item.label }}</span>
            </a-menu-item>
          </a-menu-item-group>
        </a-menu>
      </div>
      <div v-if="!collapsed || isMobile" class="wm-sider__foot">全部写操作均被审计</div>
    </aside>

    <!-- Mobile drawer -->
    <a-drawer
      v-model:open="drawerOpen"
      placement="left"
      :width="260"
      :closable="false"
      class="wm-mobile-drawer"
    >
      <div class="wm-brand wm-brand--drawer"><span class="wm-brand__mark">寒</span><span class="wm-brand__text">寒语馨面板</span></div>
      <a-menu theme="light" mode="inline" :selected-keys="[route.path]" class="wm-nav">
        <a-menu-item-group v-for="group in navGroups" :key="group.label" :title="group.label">
          <a-menu-item v-for="item in group.items" :key="item.path" @click="go(item.path)">
            <template #icon><component :is="item.icon" /></template>
            <span>{{ item.label }}</span>
          </a-menu-item>
        </a-menu-item-group>
      </a-menu>
    </a-drawer>

    <div class="wm-main">
      <header class="wm-topbar">
        <div class="wm-topbar__left">
          <button class="wm-icon-btn" type="button" :aria-label="isMobile ? '打开导航' : '切换导航'" @click="toggleNav">
            <Menu v-if="isMobile" /><Expand v-else-if="collapsed" /><Fold v-else />
          </button>
          <div class="wm-crumb">
            <span>{{ route.meta.group || '面板' }}</span>
            <span class="wm-crumb__sep">/</span>
            <span class="wm-crumb__current">{{ route.meta.title }}</span>
          </div>
          <span class="wm-topbar__title">{{ route.meta.title }}</span>
        </div>
        <div class="wm-topbar__right">
          <span class="wm-session">
            <i class="wm-session__dot" :class="{ 'is-plain': !secure }" />
            {{ secure ? '安全连接' : '明文连接' }}
          </span>
          <a-tooltip :title="theme.dark ? '切换到亮色' : '切换到暗色'">
            <button class="wm-icon-btn" type="button" @click="theme.toggle()">
              <Sunny v-if="theme.dark" /><Moon v-else />
            </button>
          </a-tooltip>
          <a-dropdown trigger="['click']">
            <button class="wm-user-trigger" type="button" aria-label="用户菜单">
              <a-avatar :size="26" class="wm-user-avatar">{{ (auth.username || 'U').slice(0, 1).toUpperCase() }}</a-avatar>
              <span class="wm-user-trigger__name">{{ auth.username }}</span>
              <ArrowDown />
            </button>
            <template #overlay>
              <a-menu class="wm-user-menu" @click="({ key }: any) => onCommand(String(key))">
                <a-menu-item disabled>当前角色：{{ roleLabel }}</a-menu-item>
                <a-menu-divider />
                <a-menu-item key="theme">
                  <Moon v-if="!theme.dark" /><Sunny v-else />&nbsp;{{ theme.dark ? '亮色外观' : '暗色外观' }}
                </a-menu-item>
                <a-menu-divider />
                <a-menu-item key="logout"><SwitchButton />&nbsp;退出登录</a-menu-item>
              </a-menu>
            </template>
          </a-dropdown>
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

    <nav class="wm-mobilebar" aria-label="快捷导航">
      <router-link
        v-for="item in mobileItems"
        :key="item.path"
        :to="item.path"
        class="wm-mobilebar__item"
        :class="{ 'is-active': route.path === item.path }"
      >
        <component :is="item.icon" /><span>{{ item.label }}</span>
      </router-link>
      <button class="wm-mobilebar__item" type="button" @click="drawerOpen = true">
        <MoreFilled /><span>更多</span>
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
} from '../antd-compat'
import { useAuth } from '../stores/auth'
import { t as tr } from '../i18n'
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
  { label: tr('概览'), items: [{ path: '/dashboard', label: tr('仪表盘'), icon: Odometer }] },
  {
    label: tr('站点与服务'),
    items: [
      { path: '/website', label: tr('网站'), icon: Connection },
      { path: '/databases', label: tr('数据库'), icon: Coin },
      { path: '/softstore', label: tr('软件商店'), icon: Shop },
      { path: '/dockerstore', label: tr('Docker 容器管理'), icon: Box },
      ...(auth.role !== 'viewer' ? [{ path: '/terminal', label: tr('终端'), icon: Monitor }] : [])
    ]
  },
  {
    label: tr('系统运维'),
    items: [
      { path: '/cron', label: tr('计划任务'), icon: Timer },
      { path: '/alerts', label: tr('告警策略'), icon: Warning },
      { path: '/files', label: tr('文件管理'), icon: Folder },
      { path: '/proc', label: tr('进程管理'), icon: Cpu },
      { path: '/nodes', label: tr('节点管理'), icon: Share },
      { path: '/firewall', label: tr('防火墙'), icon: Lock },
      { path: '/certs', label: tr('SSL 证书'), icon: Stamp }
    ]
  },
  {
    label: tr('面板'),
    items: [
      { path: '/audit', label: tr('审计日志'), icon: Document },
      { path: '/notify', label: tr('通知与存储'), icon: Bell },
      ...(auth.role === 'admin' ? [{ path: '/users', label: tr('用户管理'), icon: Avatar }] : []),
      { path: '/setting', label: tr('系统设置'), icon: Setting }
    ]
  }
])

const mobileItems = [
  { path: '/dashboard', label: tr('仪表盘'), icon: Odometer },
  { path: '/website', label: tr('网站'), icon: Connection },
  { path: '/cron', label: tr('计划任务'), icon: Timer },
  { path: '/files', label: tr('文件'), icon: Folder }
]

function go(path: string) {
  router.push(path)
  drawerOpen.value = false
}

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
