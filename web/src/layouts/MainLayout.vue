<template>
  <div class="wm-shell">
    <!-- 顶部导航栏：品牌 + 分组下拉 + 右侧动作区 -->
    <header class="wm-topbar">
      <div class="wm-topbar__brand">
        <span class="wm-brand__mark">寒</span>
        <span class="wm-brand__text">寒语馨面板</span>
      </div>

      <!-- 桌面：分组下拉导航 -->
      <nav v-if="!isMobile" class="wm-topnav" aria-label="主导航">
        <a-dropdown v-for="group in navGroups" :key="group.label" trigger="['click', 'hover']" placement="bottomLeft">
          <button
            type="button"
            class="wm-topnav__item"
            :class="{ 'is-active': group.items.some((i) => i.path === route.path) }"
          >
            {{ group.label }}<ArrowDown class="wm-topnav__caret" />
          </button>
          <template #overlay>
            <a-menu class="wm-topnav-menu" :selected-keys="[route.path]" @click="({ key }: any) => go(String(key))">
              <a-menu-item v-for="item in group.items" :key="item.path">
                <template #icon>
                  <span class="wm-nav-ico" :style="{ background: item.c.soft, color: item.c.color }"><component :is="item.icon" /></span>
                </template>
                <span>{{ item.label }}</span>
              </a-menu-item>
            </a-menu>
          </template>
        </a-dropdown>
      </nav>

      <!-- 移动端：汉堡 -->
      <button v-if="isMobile" class="wm-icon-btn wm-topnav__burger" type="button" aria-label="打开导航" @click="drawerOpen = true">
        <Menu />
      </button>

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

    <!-- 移动端抽屉（全菜单） -->
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
            <template #icon><span class="wm-nav-ico" :style="{ background: item.c.soft, color: item.c.color }"><component :is="item.icon" /></span></template>
            <span>{{ item.label }}</span>
          </a-menu-item>
        </a-menu-item-group>
      </a-menu>
    </a-drawer>

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
  Lock, Stamp, Document, Bell, Setting, Sunny, Moon,
  ArrowDown, SwitchButton, Avatar, Share, Menu, MoreFilled, Box
} from '../antd-compat'
import { useAuth } from '../stores/auth'
import { t as tr } from '../i18n'
import { useTheme } from '../stores/theme'

const auth = useAuth()
const theme = useTheme()
const route = useRoute()
const router = useRouter()

const drawerOpen = ref(false)
const isMobile = ref(false)
const secure = location.protocol === 'https:'

const ROLE_LABEL: Record<string, string> = { admin: '管理员', operator: '操作员', viewer: '只读' }
const roleLabel = computed(() => ROLE_LABEL[auth.role] || '未登录')

// 彩色模块色板（CSS 变量对：图标底色 + 前景色）
const C = {
  blue: { color: 'var(--wm-p-blue)', soft: 'var(--wm-p-blue-soft)' },
  green: { color: 'var(--wm-p-green)', soft: 'var(--wm-p-green-soft)' },
  orange: { color: 'var(--wm-p-orange)', soft: 'var(--wm-p-orange-soft)' },
  violet: { color: 'var(--wm-p-violet)', soft: 'var(--wm-p-violet-soft)' },
  cyan: { color: 'var(--wm-p-cyan)', soft: 'var(--wm-p-cyan-soft)' },
  red: { color: 'var(--wm-p-red)', soft: 'var(--wm-p-red-soft)' },
  amber: { color: 'var(--wm-p-amber)', soft: 'var(--wm-p-amber-soft)' },
  pink: { color: 'var(--wm-p-pink)', soft: 'var(--wm-p-pink-soft)' },
  indigo: { color: 'var(--wm-p-indigo)', soft: 'var(--wm-p-indigo-soft)' },
  teal: { color: 'var(--wm-p-teal)', soft: 'var(--wm-p-teal-soft)' }
}

const navGroups = computed(() => [
  { label: tr('概览'), items: [{ path: '/dashboard', label: tr('仪表盘'), icon: Odometer, c: C.blue }] },
  {
    label: tr('站点与服务'),
    items: [
      { path: '/website', label: tr('网站'), icon: Connection, c: C.green },
      { path: '/databases', label: tr('数据库'), icon: Coin, c: C.orange },
      { path: '/softstore', label: tr('软件商店'), icon: Shop, c: C.violet },
      { path: '/dockerstore', label: tr('Docker 容器管理'), icon: Box, c: C.cyan },
      ...(auth.role !== 'viewer' ? [{ path: '/terminal', label: tr('终端'), icon: Monitor, c: C.indigo }] : [])
    ]
  },
  {
    label: tr('系统运维'),
    items: [
      { path: '/cron', label: tr('计划任务'), icon: Timer, c: C.teal },
      { path: '/alerts', label: tr('告警策略'), icon: Warning, c: C.red },
      { path: '/files', label: tr('文件管理'), icon: Folder, c: C.amber },
      { path: '/proc', label: tr('进程管理'), icon: Cpu, c: C.pink },
      { path: '/nodes', label: tr('节点管理'), icon: Share, c: C.blue },
      { path: '/firewall', label: tr('防火墙'), icon: Lock, c: C.orange },
      { path: '/certs', label: tr('SSL 证书'), icon: Stamp, c: C.violet }
    ]
  },
  {
    label: tr('面板'),
    items: [
      { path: '/audit', label: tr('审计日志'), icon: Document, c: C.indigo },
      { path: '/notify', label: tr('通知与存储'), icon: Bell, c: C.cyan },
      ...(auth.role === 'admin' ? [{ path: '/users', label: tr('用户管理'), icon: Avatar, c: C.green }] : []),
      { path: '/setting', label: tr('系统设置'), icon: Setting, c: C.amber }
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
