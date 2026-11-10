// 多标签终端会话：一个标签 = 一个 WS + 一个 xterm 实例。
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { ClipboardAddon } from '@xterm/addon-clipboard'
import '@xterm/xterm/css/xterm.css'

export const THEMES: Record<string, { bg: string; fg: string; cursor: string; sel: string; name: string }> = {
  dark: { bg: '#14181f', fg: '#d6deeb', cursor: '#7fd1ff', sel: '#3b5c8f', name: '暗色' },
  light: { bg: '#ffffff', fg: '#263238', cursor: '#0b57d0', sel: '#b3d1f7', name: '亮色' },
  dracula: { bg: '#282a36', fg: '#f8f8f2', cursor: '#ff79c6', sel: '#44475a', name: 'Dracula' },
  nord: { bg: '#2e3440', fg: '#d8dee9', cursor: '#88c0d0', sel: '#434c5e', name: 'Nord' },
  solarized: { bg: '#002b36', fg: '#93a1a1', cursor: '#eee8d5', sel: '#073642', name: 'Solarized' }
}

export type TabMsg = { type: string; data?: string; cols?: number; rows?: number; host?: string; port?: number; username?: string; auth_type?: string; password?: string; private_key?: string; passphrase?: string }

export interface HostInfo {
  id: number
  name: string
  host: string
  port: number
  username: string
  auth_type: 'password' | 'key'
  has_password: boolean
  has_key: boolean
  sort: number
}

let seq = 0

export class TermTab {
  id = ++seq
  title: string
  hostId: number | null // null = 快速连接/本机
  hostLabel: string
  term: Terminal
  fit: FitAddon
  ws: WebSocket | null = null
  status: 'connecting' | 'connected' | 'closed' = 'connecting'
  lastInput = Date.now()
  idleWarned = false
  onClose: (id: number) => void = () => {}
  private container: HTMLElement | null = null
  private ro: ResizeObserver | null = null

  constructor(opts: { title: string; hostId: number | null; hostLabel: string; theme: string; fontSize: number }) {
    this.title = opts.title
    this.hostId = opts.hostId
    this.hostLabel = opts.hostLabel
    const th = THEMES[opts.theme] || THEMES.dark
    this.term = new Terminal({
      cursorBlink: true,
      fontSize: opts.fontSize,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace',
      scrollback: 5000,
      allowProposedApi: true,
      theme: { background: th.bg, foreground: th.fg, cursor: th.cursor, selectionBackground: th.sel }
    })
    this.fit = new FitAddon()
    this.term.loadAddon(this.fit)
    this.term.loadAddon(new ClipboardAddon())
    // 选中自动复制
    this.term.onSelectionChange(() => {
      const sel = this.term.getSelection()
      if (sel && navigator.clipboard) navigator.clipboard.writeText(sel).catch(() => {})
    })
    this.term.onData((d) => {
      this.send({ type: 'input', data: d })
      this.lastInput = Date.now()
      this.idleWarned = false
    })
    this.term.attachCustomKeyEventHandler((ev) => {
      // Ctrl+Shift 组合键交给全局（复制/粘贴/全屏/清屏）
      if (ev.type === 'keydown' && ev.ctrlKey && ev.shiftKey && ['v', 'f', 'k'].includes(ev.key.toLowerCase())) return false
      return true
    })
  }

  mount(el: HTMLElement) {
    this.container = el
    this.term.open(el)
    this.ro = new ResizeObserver(() => this.fitNow())
    this.ro.observe(el)
    this.fitNow()
  }

  fitNow() {
    try {
      this.fit.fit()
      if (this.ws && this.ws.readyState === WebSocket.OPEN) {
        this.send({ type: 'resize', cols: this.term.cols, rows: this.term.rows })
      }
    } catch { /* ignore */ }
  }

  connect(cred?: TabMsg) {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    let url = `${proto}://${location.host}/api/ws/ssh?cols=${this.term.cols || 120}&rows=${this.term.rows || 32}`
    if (this.hostId) url += `&host_id=${this.hostId}`
    this.ws = new WebSocket(url)
    this.ws.onopen = () => {
      // 快速连接：首条消息发凭据（服务端等这条再拨号）
      if (!this.hostId && cred) {
        this.ws!.send(JSON.stringify({ type: 'cred', ...cred }))
      }
      this.status = 'connected'
      this.lastInput = Date.now()
    }
    this.ws.onmessage = (ev) => {
      // 服务端控制帧是 JSON，pty 输出是原始文本
      if (typeof ev.data === 'string' && ev.data.startsWith('{"type":')) {
        try {
          const m = JSON.parse(ev.data) as TabMsg
          if (m.type === 'connected') {
            this.term.writeln('\x1b[32m[已连接 ' + (m.data || '') + ']\x1b[0m')
            return
          }
          if (m.type === 'idle') {
            this.idleWarned = true
            this.onIdle(m.data || '')
            return
          }
          if (m.type === 'exit') {
            this.term.writeln('\r\n\x1b[31m[' + (m.data || '连接已关闭') + ']\x1b[0m')
            this.status = 'closed'
            this.onClose(this.id)
            return
          }
        } catch { /* fallthrough 原始输出 */ }
      }
      this.term.write(ev.data)
    }
    this.ws.onclose = () => {
      if (this.status !== 'closed') {
        this.status = 'closed'
        this.term.writeln('\r\n\x1b[33m[连接已断开]\x1b[0m')
        this.onClose(this.id)
      }
    }
  }

  onIdle: (msg: string) => void = () => {}
  keepAlive() {
    // 用户点了「继续」：重置空闲计时（发一个无效输入由服务端刷新 lastInput）
    this.lastInput = Date.now()
    this.idleWarned = false
    this.send({ type: 'input', data: '' })
  }

  send(obj: TabMsg) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify(obj))
  }

  paste(text: string) {
    if (text) this.send({ type: 'input', data: text })
    this.term.focus()
  }

  destroy() {
    try { this.ro?.disconnect() } catch { /* ignore */ }
    try { this.ws?.close() } catch { /* ignore */ }
    try { this.term.dispose() } catch { /* ignore */ }
  }
}
