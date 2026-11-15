<template>
  <div class="wm-page term-page">
    <!-- 主机收藏栏 -->
    <WmSectionCard title="SSH 主机" description="云端收藏的主机凭据（AES 加密存储），点击即开新标签连接；支持上/下移排序" marker dense>
      <template #actions>
        <a-button size="small" @click="openQuick">快速连接</a-button>
        <a-button size="small" type="primary" @click="openAddHost">新增主机</a-button>
      </template>
      <div class="host-list">
        <!-- 本机：面板所在服务器，本地 Shell 无需凭据 -->
        <div class="host-item host-item--local">
          <div class="host-main" @click="openLocalTab">
            <span class="host-name">本机（面板所在服务器）</span>
            <WmStatusPill label="本地 Shell" tone="success" :dot="false" />
          </div>
        </div>
        <div v-for="(h, i) in hosts" :key="h.id" class="host-item">
          <div class="host-main" @click="openHostTab(h)">
            <span class="host-name">{{ h.name || h.host }}</span>
            <span class="host-meta wm-mono">{{ h.username }}@{{ h.host }}:{{ h.port }}</span>
            <WmStatusPill :label="h.auth_type === 'key' ? '私钥' : '密码'" tone="info" :dot="false" />
          </div>
          <div class="host-ops">
            <a-button size="small" type="text" :disabled="i === 0" @click.stop="moveHost(i, -1)" title="上移">↑</a-button>
            <a-button size="small" type="text" :disabled="i === hosts.length - 1" @click.stop="moveHost(i, 1)" title="下移">↓</a-button>
            <a-button size="small" type="text" @click.stop="openEditHost(h)">编辑</a-button>
            <a-button size="small" type="text" danger @click.stop="removeHost(h)">删除</a-button>
          </div>
        </div>
        <WmEmptyState v-if="!hosts.length" title="还没有收藏主机" description="点击「新增主机」收藏常用 SSH；「快速连接」可不保存临时使用；本机 Shell 始终可用" />
      </div>
    </WmSectionCard>

    <!-- 终端多标签 -->
    <WmSectionCard title="Web 终端" description="多标签同时管理多台服务器；会话经登录态鉴权，操作全部审计" marker :dense="false">
      <template #actions>
        <a-space :size="6" wrap>
          <a-select v-model:value="themeName" size="small" style="width: 110px" :options="themeOptions" @change="applyTheme" />
          <a-button size="small" @click="copySel" title="Ctrl+Shift+C">复制</a-button>
          <a-button size="small" @click="pasteClip" title="Ctrl+Shift+V / 右键">粘贴</a-button>
          <a-button size="small" @click="clearTerm">清屏</a-button>
          <a-button size="small" @click="fontDown">A-</a-button>
          <a-button size="small" @click="fontUp">A+</a-button>
        </a-space>
      </template>

      <!-- 标签条 -->
      <div class="tabbar">
        <div v-for="t in tabs" :key="t.id" class="tab" :class="{ active: t.id === activeId }" @click="activate(t.id)">
          <span class="tab-dot" :class="'s-' + (statusMap[t.id] || t.status)"></span>
          <span class="tab-title">{{ t.title }}</span>
          <span class="tab-close" @click.stop="closeTab(t.id)">×</span>
        </div>
        <div v-if="!tabs.length" class="tabbar-empty">没有打开的终端；点击上方主机或「快速连接」开始</div>
      </div>

      <!-- 终端区（所有标签 DOM 常驻，切标签时 display 切换保输出） -->
      <div class="term-area" :style="{ height: termHeight }">
        <div v-for="t in tabs" :key="t.id" v-show="t.id === activeId" class="term-pane" :style="{ background: paneBg }" :ref="el => setPane(t.id, el)"></div>
        <div v-if="!tabs.length" class="term-empty">
          <p>从上方选择主机连接，或使用「快速连接」</p>
          <p class="wm-text-muted">支持多标签并行 · 选中即复制 · 右键粘贴 · Ctrl+Shift+F 全屏</p>
        </div>
      </div>

      <!-- 移动端/触屏快捷键条 -->
      <div class="keybar">
        <button v-for="k in keyItems" :key="k.label" class="key-btn" :title="k.title" @click="k.run">{{ k.label }}</button>
        <button class="key-btn key-btn--add" @click="openKeysModal" title="自定义快捷键条"><span class="wm-icon"><Setting /></span></button>
      </div>
      <p class="term-tip">
        选中自动复制 · 右键复制/粘贴 · Ctrl+Shift+C/V 复制粘贴 · Ctrl+Shift+F 全屏 · Ctrl+Shift+K 清屏 · 空闲 10 分钟提醒、15 分钟自动断开
      </p>
    </WmSectionCard>

    <!-- 快速连接 / 主机编辑 抽屉 -->
    <a-drawer v-model:open="dlgOpen" :title="dlgTitle" :width="680">
      <a-form layout="vertical">
        <div class="wm-form-sec">
          <div class="wm-form-sec__title">连接信息</div>
          <a-form-item label="类型">
            <a-radio-group v-model:value="dlg.mode" :disabled="dlg.mode === 'edit'">
              <a-radio value="quick">临时连接（不保存）</a-radio>
              <a-radio value="new">保存为收藏</a-radio>
              <a-radio v-if="dlg.mode === 'edit'" value="edit">编辑收藏</a-radio>
            </a-radio-group>
          </a-form-item>
          <a-form-item v-if="dlg.mode !== 'quick'" label="备注名"><a-input v-model:value="dlg.name" placeholder="如：生产 Web" /></a-form-item>
          <a-form-item label="主机"><a-input v-model:value="dlg.host" placeholder="IP 或域名" /></a-form-item>
          <a-form-item label="端口"><a-input-number v-model:value="dlg.port" :min="1" :max="65535" style="width: 140px" /></a-form-item>
          <a-form-item label="用户名" style="margin-bottom: 0"><a-input v-model:value="dlg.username" placeholder="root" /></a-form-item>
        </div>
        <div class="wm-form-sec">
          <div class="wm-form-sec__title">认证</div>
          <a-form-item label="认证方式">
            <a-radio-group v-model:value="dlg.authType">
              <a-radio value="password">密码</a-radio>
              <a-radio value="key">私钥</a-radio>
            </a-radio-group>
          </a-form-item>
          <a-form-item v-if="dlg.authType === 'password'" label="密码" style="margin-bottom: 0">
            <a-input-password v-model:value="dlg.password" :placeholder="dlg.mode === 'edit' ? '留空表示不修改' : ''" />
          </a-form-item>
          <template v-else>
            <a-form-item label="私钥（PEM）">
              <a-textarea v-model:value="dlg.privateKey" :rows="4" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" />
            </a-form-item>
            <a-form-item label="私钥口令（可选）" style="margin-bottom: 0"><a-input-password v-model:value="dlg.passphrase" /></a-form-item>
          </template>
          <WmNotice v-if="dlg.mode === 'quick'" tone="warning" class="wm-mt-4">临时连接的凭据只在本次会话内存中使用，不落库、不保存。</WmNotice>
        </div>
      </a-form>
      <template #footer>
        <a-button @click="dlgOpen = false">取消</a-button>
        <a-button type="primary" data-testid="dlg-ok" @click="dlgOk">{{ dlg.mode === 'quick' ? '连接' : '保存' }}</a-button>
      </template>
    </a-drawer>

    <!-- 自定义快捷键条 -->
    <a-modal v-model:open="keysOpen" title="自定义快捷键条" ok-text="保存" cancel-text="取消" @ok="saveKeys">
      <p class="wm-text-muted">每行一个按钮，格式 <code>显示文本:发送内容</code>，支持 \t \e（Esc）等转义；留发送内容则只显示不可发。</p>
      <a-textarea v-model:value="keysDraft" :rows="8" class="wm-mono" />
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, Setting } from '../antd-compat'
import api from '../api'
import { TermTab, THEMES, type HostInfo, type TabMsg } from './term-session'

const hosts = ref<HostInfo[]>([])
const tabs = ref<TermTab[]>([])
const activeId = ref(0)
const statusMap = reactive<Record<number, string>>({})
const themeName = ref(localStorage.getItem('term_theme') || 'dark')
const fontSize = ref(14)
const fullscreen = ref(false)
const termHeight = computed(() => (fullscreen.value ? 'calc(100vh - 320px)' : '52vh'))
const panes = new Map<number, HTMLElement>()
const themeOptions = Object.entries(THEMES).map(([k, v]) => ({ value: k, label: v.name }))
// 终端容器底色随所选主题（与 xterm 实例自身主题底色一致）
const paneBg = computed(() => (THEMES[themeName.value] || THEMES.dark).bg)

function setPane(id: number, el: any) {
  if (el) panes.set(id, el as HTMLElement)
  else panes.delete(id)
}

// ---- 快捷键条（可自定义，localStorage 持久化） ----
const DEFAULT_KEYS = 'Esc:\\e\nTab:\\t\nCtrl+C:\\x03\nCtrl+D:\\x04\nCtrl+U:\\x15\nCtrl+W:\\x17\n↑:\\e[A\n↓:\\e[B\n←:\\e[D\n→:\\e[C'
const keyItems = ref<{ label: string; title?: string; seq?: string; run: () => void }[]>([])
const keysOpen = ref(false)
const keysDraft = ref('')

function parseKeys(raw: string) {
  const items: { label: string; title?: string; seq?: string; run: () => void }[] = []
  for (const line of raw.split('\n')) {
    const s = line.trim()
    if (!s) continue
    const idx = s.indexOf(':')
    if (idx <= 0) continue
    const label = s.slice(0, idx).trim()
    let seq = s.slice(idx + 1)
    if (seq === undefined) seq = ''
    const decoded = seq.replace(/\\t/g, '\t').replace(/\\e/g, '\x1b').replace(/\\x([0-9a-fA-F]{2})/g, (_, h) => String.fromCharCode(parseInt(h, 16))).replace(/\\n/g, '\n')
    items.push({
      label,
      title: seq || undefined,
      seq: seq === '' ? undefined : decoded,
      run: () => {
        const t = activeTab()
        if (!t) return
        if (decoded !== '') t.send({ type: 'input', data: decoded })
        t.term.focus()
      }
    })
  }
  return items
}

function loadKeys() {
  const raw = localStorage.getItem('term_keys') || DEFAULT_KEYS
  keysDraft.value = raw
  keyItems.value = parseKeys(raw)
}

function openKeysModal() {
  keysOpen.value = true
}

function saveKeys() {
  localStorage.setItem('term_keys', keysDraft.value)
  keyItems.value = parseKeys(keysDraft.value)
  keysOpen.value = false
  ElMessage.success('快捷键条已保存')
}

// ---- 空闲提醒 ----
async function handleIdle(t: TermTab, msg: string) {
  try {
    await ElMessageBox.confirm(msg, '会话空闲', { type: 'warning' })
    t.keepAlive()
    ElMessage.success('会话已保持')
  } catch {
    t.send({ type: 'input', data: 'exit\n' })
  }
}

// ---- 标签管理 ----
function activeTab() {
  return tabs.value.find((t) => t.id === activeId.value) || null
}

function activate(id: number) {
  activeId.value = id
  const t = tabs.value.find((x) => x.id === id)
  if (t) {
    requestAnimationFrame(() => {
      t.fitNow()
      t.term.focus()
    })
  }
}

function addTab(t: TermTab) {
  tabs.value.push(t)
  statusMap[t.id] = t.status
  t.onStatus = (id, s) => {
    statusMap[id] = s
  }
  t.onClose = (id) => {
    /* 保留标签显示断开状态，由用户手动关闭 */
  }
  t.onIdle = (msg) => handleIdle(t, msg)
  activeId.value = t.id
  requestAnimationFrame(() => {
    const el = panes.get(t.id)
    if (el && !el.dataset.mounted) {
      t.mount(el)
      el.dataset.mounted = '1'
    }
    t.fitNow()
    t.connect(t.pendingCred)
    t.term.focus()
  })
}

function closeTab(id: number) {
  const t = tabs.value.find((x) => x.id === id)
  if (t) t.destroy()
  delete statusMap[id]
  tabs.value = tabs.value.filter((x) => x.id !== id)
  if (activeId.value === id) activeId.value = tabs.value.length ? tabs.value[tabs.value.length - 1].id : 0
}

// ---- 连接入口 ----
declare module './term-session' {
  interface TermTab {
    pendingCred?: TabMsg
  }
}

function openHostTab(h: HostInfo) {
  const t = new TermTab({ title: h.name || h.host, hostId: h.id, hostLabel: h.username + '@' + h.host, theme: themeName.value, fontSize: fontSize.value })
  addTab(t)
}

// 本机 Shell（面板所在服务器，无需凭据）
function openLocalTab() {
  const t = new TermTab({ title: '本机', hostId: null, hostLabel: 'local', theme: themeName.value, fontSize: fontSize.value, mode: 'local' })
  addTab(t)
}

let dlg = reactive({ mode: 'quick' as 'quick' | 'new' | 'edit', id: 0, name: '', host: '', port: 22, username: 'root', authType: 'password' as 'password' | 'key', password: '', privateKey: '', passphrase: '' })
const dlgOpen = ref(false)
const dlgTitle = computed(() => (dlg.mode === 'edit' ? '编辑主机' : dlg.mode === 'quick' ? '快速连接（不保存）' : '新增收藏主机'))

function openQuick() {
  Object.assign(dlg, { mode: 'quick', id: 0, name: '', host: '', port: 22, username: 'root', authType: 'password', password: '', privateKey: '', passphrase: '' })
  dlgOpen.value = true
}

function openAddHost() {
  Object.assign(dlg, { mode: 'new', id: 0, name: '', host: '', port: 22, username: 'root', authType: 'password', password: '', privateKey: '', passphrase: '' })
  dlgOpen.value = true
}

function openEditHost(h: HostInfo) {
  Object.assign(dlg, { mode: 'edit', id: h.id, name: h.name, host: h.host, port: h.port, username: h.username, authType: h.auth_type, password: '', privateKey: '', passphrase: '' })
  dlgOpen.value = true
}

async function dlgOk() {
  if (!dlg.host || !dlg.username) {
    ElMessage.warning('请填写主机与用户名')
    return
  }
  if (dlg.mode === 'quick') {
    const cred: TabMsg = { type: 'cred', host: dlg.host, port: dlg.port, username: dlg.username, auth_type: dlg.authType, password: dlg.password, private_key: dlg.privateKey, passphrase: dlg.passphrase }
    const t = new TermTab({ title: dlg.host, hostId: null, hostLabel: dlg.username + '@' + dlg.host, theme: themeName.value, fontSize: fontSize.value })
    t.pendingCred = cred
    addTab(t)
    dlgOpen.value = false
    return
  }
  const payload: any = { name: dlg.name, host: dlg.host, port: dlg.port, username: dlg.username, auth_type: dlg.authType }
  if (dlg.authType === 'password') payload.password = dlg.password
  else {
    payload.private_key = dlg.privateKey
    payload.passphrase = dlg.passphrase
  }
  if (dlg.mode === 'edit') {
    await api.put('/term/hosts/' + dlg.id, payload)
  } else {
    await api.post('/term/hosts', payload)
  }
  dlgOpen.value = false
  ElMessage.success('已保存')
  loadHosts()
}

// ---- 主机 CRUD / 排序 ----
async function loadHosts(): Promise<HostInfo[]> {
  try {
    hosts.value = (await api.get('/term/hosts')) as any
  } catch {
    hosts.value = []
  }
  return hosts.value
}

async function moveHost(i: number, d: number) {
  const arr = [...hosts.value]
  const [it] = arr.splice(i, 1)
  arr.splice(i + d, 0, it)
  hosts.value = arr
  await api.post('/term/hosts/reorder', { ids: arr.map((h) => h.id) })
}

async function removeHost(h: HostInfo) {
  try {
    await ElMessageBox.confirm(`确认删除收藏「${h.name || h.host}」？`, '删除', { type: 'warning' })
  } catch {
    return
  }
  await api.delete('/term/hosts/' + h.id)
  loadHosts()
}

// ---- 终端操作 ----
function copySel() {
  const t = activeTab()
  if (!t) return
  const sel = t.term.getSelection()
  if (sel && navigator.clipboard) navigator.clipboard.writeText(sel).catch(() => {})
}

async function pasteClip() {
  const t = activeTab()
  if (!t) return
  try {
    const text = await navigator.clipboard.readText()
    t.paste(text)
  } catch {
    ElMessage.warning('浏览器拒绝了剪贴板权限，请使用 Ctrl+Shift+V 或右键粘贴')
  }
}

function clearTerm() {
  activeTab()?.term.clear()
}

function fontUp() {
  if (fontSize.value >= 28) return
  fontSize.value++
  applyFont()
}

function fontDown() {
  if (fontSize.value <= 10) return
  fontSize.value--
  applyFont()
}

function applyFont() {
  for (const t of tabs.value) {
    t.term.options.fontSize = fontSize.value
    t.fitNow()
  }
}

function applyTheme() {
  localStorage.setItem('term_theme', themeName.value)
  const th = THEMES[themeName.value]
  for (const t of tabs.value) t.term.options.theme = { background: th.bg, foreground: th.fg, cursor: th.cursor, selectionBackground: th.sel }
}

function onKey(e: KeyboardEvent) {
  if (!e.ctrlKey || !e.shiftKey) return
  const k = e.key.toLowerCase()
  if (k === 'v') {
    e.preventDefault()
    pasteClip()
  } else if (k === 'c') {
    const t = activeTab()
    if (t?.term.hasSelection()) e.preventDefault()
  } else if (k === 'f') {
    e.preventDefault()
    fullscreen.value = !fullscreen.value
    requestAnimationFrame(() => activeTab()?.fitNow())
  } else if (k === 'k') {
    e.preventDefault()
    clearTerm()
  } else if (/^[1-9]$/.test(k) && e.altKey) {
    // Ctrl+Shift+1..9 切标签
    e.preventDefault()
    const idx = parseInt(k) - 1
    if (tabs.value[idx]) activate(tabs.value[idx].id)
  }
}

onMounted(async () => {
  window.addEventListener('keydown', onKey)
  loadKeys()
  // 默认连接：有收藏主机时自动连第一台（SSH）；否则默认连本机 Shell
  const list = await loadHosts()
  if (list.length) openHostTab(list[0])
  else openLocalTab()
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  for (const t of tabs.value) t.destroy()
})
</script>

<style scoped>
/* 主机收藏栏 */
.host-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.host-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 8px 12px;
  border: 1px solid var(--wm-line);
  border-radius: var(--wm-r-lg);
  background: var(--wm-surface);
  transition: border-color var(--wm-dur-fast) var(--wm-ease);
}
.host-item:hover {
  border-color: var(--wm-line-strong);
}
.host-item--local {
  border-left: 3px solid var(--wm-primary);
}
.host-main {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  min-width: 0;
}
.host-main:hover .host-name {
  color: var(--wm-primary);
}
.host-name {
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.host-meta {
  color: var(--wm-ink-3);
  font-size: 12px;
  white-space: nowrap;
}
.host-ops {
  display: flex;
  gap: 2px;
  flex-shrink: 0;
}
/* 标签条 */
.tabbar {
  display: flex;
  gap: 4px;
  align-items: center;
  flex-wrap: nowrap;
  overflow-x: auto;
  padding: 4px 2px 0;
  border-bottom: 1px solid var(--wm-line);
  margin-bottom: 8px;
}
.tab {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 10px;
  border-radius: var(--wm-r-md) var(--wm-r-md) 0 0;
  cursor: pointer;
  font-size: 13px;
  color: var(--wm-ink-2);
  white-space: nowrap;
  border: 1px solid transparent;
  border-bottom: 0;
  user-select: none;
}
.tab:hover {
  background: var(--wm-surface-3);
}
.tab.active {
  background: var(--wm-surface);
  border-color: var(--wm-line);
  color: var(--wm-ink);
  font-weight: 600;
}
.tab-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}
.s-connecting {
  background: var(--wm-warning);
}
.s-connected {
  background: var(--wm-success);
}
.s-closed {
  background: var(--wm-ink-3);
}
.tab-close {
  opacity: 0.55;
  padding: 0 2px;
  border-radius: var(--wm-r-sm);
}
.tab-close:hover {
  opacity: 1;
  background: var(--wm-surface-3);
}
.tabbar-empty {
  color: var(--wm-ink-3);
  font-size: 13px;
  padding: 6px;
}
/* 终端区 */
.term-area {
  position: relative;
  min-height: 300px;
  border-radius: var(--wm-r-lg);
  overflow: hidden;
}
.term-pane {
  width: 100%;
  height: 100%;
  padding: 8px 10px;
  border-radius: var(--wm-r-lg);
  }
.term-empty {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border-radius: var(--wm-r-lg);
  background: var(--wm-surface-2);
  color: var(--wm-ink-3);
}
.term-tip {
  margin-top: 8px;
  font-size: 12px;
  color: var(--wm-ink-3);
}
/* 快捷键条（移动端友好：大按钮、横向滚动） */
.keybar {
  display: flex;
  gap: 6px;
  overflow-x: auto;
  margin-top: 10px;
  padding: 4px 0;
  -webkit-overflow-scrolling: touch;
}
.key-btn {
  flex-shrink: 0;
  min-width: 52px;
  padding: 8px 12px;
  border-radius: var(--wm-r-md);
  border: 1px solid var(--wm-line);
  background: var(--wm-surface);
  color: var(--wm-ink-2);
  font-family: var(--wm-font-mono);
  font-size: 13px;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.key-btn:active {
  background: var(--wm-primary);
  color: var(--wm-primary-contrast);
}
.key-btn--add {
  min-width: 40px;
}

</style>
