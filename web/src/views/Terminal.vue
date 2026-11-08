<template>
  <div class="wm-page">
    <WmSectionCard title="Web 终端" description="会话经登录态鉴权，全部操作将被审计记录" marker dense>
      <WmToolbar>
        <WmStatusPill :label="connected ? '已连接' : '未连接'" :tone="connected ? 'success' : 'danger'" :pulse="connected" />
        <a-button size="small" @click="connect">重新连接</a-button>
        <template #note>终端尺寸随窗口自适应；连接断开后可重新连接。</template>
      </WmToolbar>
      <div ref="box" class="term-box"></div>
    </WmSectionCard>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'

const box = ref<HTMLElement>()
const connected = ref(false)
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
  }
  ws.onmessage = (ev) => term?.write(ev.data)
  ws.onclose = () => {
    connected.value = false
    term?.writeln('\r\n\x1b[33m[连接已断开，可点击“重新连接”]\x1b[0m')
  }
}

onMounted(() => {
  term = new Terminal({ cursorBlink: true, fontSize: 14, theme: { background: '#14181f' } })
  fit = new FitAddon()
  term.loadAddon(fit)
  if (box.value) {
    term.open(box.value)
    ro = new ResizeObserver(() => doFit())
    ro.observe(box.value)
  }
  term.onData((d) => send({ type: 'input', data: d }))
  term.writeln('\x1b[36mwebmblx web terminal\x1b[0m  正在连接...')
  connect()
})

onBeforeUnmount(() => {
  disconnect()
  ro?.disconnect()
  term?.dispose()
})
</script>
