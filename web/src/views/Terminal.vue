<template>
  <div class="wm-page" :class="{ 'term-fullscreen': fullscreen }">
    <WmSectionCard title="Web 终端" description="会话经登录态鉴权，全部操作将被审计记录" marker :dense="false">
      <template #actions>
        <a-space :size="6" wrap>
          <WmStatusPill :label="connected ? '已连接' : '未连接'" :tone="connected ? 'success' : 'danger'" :pulse="connected" />
          <a-button size="small" @click="copySel" title="复制选中内容（Ctrl+Shift+C）">复制</a-button>
          <a-button size="small" @click="pasteClip" title="粘贴剪贴板（Ctrl+Shift+V / 右键）">粘贴</a-button>
          <a-button size="small" @click="clearTerm" title="清屏（Ctrl+L 发送清屏，Ctrl+Shift+K 清缓冲）">清屏</a-button>
          <a-button size="small" @click="fontDown" title="减小字号">A-</a-button>
          <a-button size="small" @click="fontUp" title="增大字号">A+</a-button>
          <a-button size="small" @click="toggleFullscreen" :title="fullscreen ? '退出全屏（Ctrl+Shift+F）' : '全屏（Ctrl+Shift+F）'">
            {{ fullscreen ? '退出全屏' : '全屏' }}
          </a-button>
          <a-button size="small" type="primary" @click="connect">重新连接</a-button>
        </a-space>
      </template>

      <div class="term-wrap" :style="{ height: termHeight }">
        <div ref="box" class="term-box"></div>
      </div>
      <p class="term-tip">
        选中文字自动复制；右键 / Ctrl+Shift+V 粘贴；Ctrl+Shift+C 复制；Ctrl+Shift+F 全屏。终端尺寸随窗口自适应，连接断开后可重新连接。
      </p>
    </WmSectionCard>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { ClipboardAddon } from '@xterm/addon-clipboard'
import '@xterm/xterm/css/xterm.css'

const box = ref<HTMLElement>()
const connected = ref(false)
const fullscreen = ref(false)
const fontSize = ref(14)
const termHeight = computed(() => (fullscreen.value ? 'calc(100vh - 220px)' : '62vh'))

let term: Terminal | null = null
let fit: FitAddon | null = null
let ws: WebSocket | null = null
let ro: ResizeObserver | null = null

function send(obj: any) {
  if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(obj))
}

function doFit() {
  if (!fit || !term) return
  try {
    fit.fit()
    send({ type: 'resize', cols: term.cols, rows: term.rows })
  } catch {
    /* ignore */
  }
}

// 选中即复制（有选区时）；浏览器不支持 clipboard API 时静默降级
async function copySel() {
  const sel = term?.getSelection()
  if (!sel) return
  try {
    await navigator.clipboard.writeText(sel)
    term?.write('')
  } catch {
    // 降级：xterm 内建复制快捷键 Ctrl+Shift+C 仍可用
  }
}

async function pasteClip() {
  try {
    const t = await navigator.clipboard.readText()
    if (t) send({ type: 'input', data: t })
  } catch {
    term?.writeln('\r\n\x1b[33m浏览器拒绝了剪贴板权限，请用右键或 Ctrl+Shift+V 粘贴\x1b[0m')
  }
}

function clearTerm() {
  term?.clear()
  term?.focus()
}

function fontUp() {
  if (fontSize.value >= 28) return
  fontSize.value += 1
  applyFont()
}

function fontDown() {
  if (fontSize.value <= 10) return
  fontSize.value -= 1
  applyFont()
}

function applyFont() {
  if (!term) return
  term.options.fontSize = fontSize.value
  doFit()
}

function toggleFullscreen() {
  fullscreen.value = !fullscreen.value
  requestAnimationFrame(() => doFit())
}

function disconnect() {
  if (ws) {
    ws.onclose = null
    ws.close()
    ws = null
  }
  connected.value = false
}

function connect() {
  disconnect()
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  ws = new WebSocket(`${proto}://${location.host}/api/ws/terminal`)
  ws.onopen = () => {
    connected.value = true
    doFit()
    term?.focus()
  }
  ws.onmessage = (ev) => term?.write(ev.data)
  ws.onclose = () => {
    connected.value = false
    term?.writeln('\r\n\x1b[33m[连接已断开，可点击“重新连接”]\x1b[0m')
  }
}

function onKey(e: KeyboardEvent) {
  if (!e.ctrlKey || !e.shiftKey) return
  const k = e.key.toLowerCase()
  if (k === 'v') {
    e.preventDefault()
    pasteClip()
  } else if (k === 'c') {
    // xterm 已处理带选区的 Ctrl+Shift+C；无选区时不拦截
    if (term?.hasSelection()) e.preventDefault()
  } else if (k === 'f') {
    e.preventDefault()
    toggleFullscreen()
  } else if (k === 'k') {
    e.preventDefault()
    clearTerm()
  }
}

onMounted(() => {
  term = new Terminal({
    cursorBlink: true,
    fontSize: fontSize.value,
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace',
    scrollback: 5000,
    allowProposedApi: true,
    theme: { background: '#14181f', selectionBackground: '#3b5c8f' }
  })
  fit = new FitAddon()
  term.loadAddon(fit)
  term.loadAddon(new ClipboardAddon())
  if (box.value) {
    term.open(box.value)
    ro = new ResizeObserver(() => doFit())
    ro.observe(box.value)
  }
  // 选中自动复制（离开鼠标即复制，像 PuTTY/Windows Terminal 常见习惯）
  term.onSelectionChange(() => {
    const sel = term?.getSelection()
    if (sel) navigator.clipboard.writeText(sel).catch(() => {})
  })
  // 右键粘贴（阻止浏览器菜单）
  box.value?.addEventListener('contextmenu', (e) => {
    e.preventDefault()
    if (term?.hasSelection()) {
      // 有选区时右键 = 复制（Windows 终端习惯），再点一次右键粘贴
      navigator.clipboard.writeText(term.getSelection()).catch(() => {})
      term.clearSelection()
    } else {
      pasteClip()
    }
  })
  term.onData((d) => send({ type: 'input', data: d }))
  term.attachCustomKeyEventHandler((ev) => {
    // 让全局 Ctrl+Shift 组合键走 window 处理器
    if (ev.type === 'keydown' && ev.ctrlKey && ev.shiftKey && ['v', 'f', 'k'].includes(ev.key.toLowerCase())) return false
    return true
  })
  window.addEventListener('keydown', onKey)
  term.writeln('\x1b[36m寒语馨面板 Web 终端\x1b[0m  正在连接...')
  connect()
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKey)
  disconnect()
  ro?.disconnect()
  term?.dispose()
})
</script>

<style scoped>
.term-wrap {
  min-height: 320px;
  border-radius: 8px;
  overflow: hidden;
  background: #14181f;
  padding: 8px 10px;
}
.term-box {
  width: 100%;
  height: 100%;
}
.term-tip {
  margin-top: 8px;
  font-size: 12px;
  color: var(--wm-text-muted, #8a94a6);
}
/* 全屏模式：卡片撑满视口 */
.term-fullscreen {
  position: fixed;
  inset: 0;
  z-index: 1000;
  background: var(--wm-bg, #f5f6f8);
  overflow: auto;
  padding: 12px;
}
</style>
