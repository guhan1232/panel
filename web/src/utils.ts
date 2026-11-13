export function fmtBytes(n?: number | null): string {
  if (n == null || isNaN(n)) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

export function fmtDuration(sec?: number | null): string {
  if (sec == null || sec < 0) return '-'
  const d = Math.floor(sec / 86400)
  const h = Math.floor((sec % 86400) / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = Math.floor(sec % 60)
  if (d > 0) return `${d}天${h}小时`
  if (h > 0) return `${h}小时${m}分钟`
  if (m > 0) return `${m}分钟${s}秒`
  return `${s}秒`
}

export function fmtCost(ms?: number | null): string {
  if (ms == null) return '-'
  if (ms < 1000) return `${ms} ms`
  return `${(ms / 1000).toFixed(1)} s`
}

/**
 * Resolve chart colors from the live design tokens so ECharts follows the
 * active light / dark theme instead of hard-coded legacy hex values.
 */
export function chartPalette() {
  const s = getComputedStyle(document.documentElement)
  const read = (name: string, fallback: string) => s.getPropertyValue(name).trim() || fallback
  return {
    accent: read('--wm-primary', '#18181b'),
    success: read('--wm-success', '#059669'),
    warning: read('--wm-warning', '#d97706'),
    muted: read('--wm-ink-3', '#a1a1aa'),
    line: read('--wm-line', '#e4e4e7'),
    surface: read('--wm-surface', '#ffffff'),
    ink: read('--wm-ink', '#18181b')
  }
}

// 终端/代码块深色底（CSS 变量运行时读取，供 xterm theme 使用）
export function codeBg(): string {
  return getComputedStyle(document.documentElement).getPropertyValue('--wm-code-bg').trim() || '#10162b'
}

export function fmtTime(t?: string | number | null): string {
  if (!t) return '-'
  const d = new Date(t)
  if (isNaN(d.getTime())) return '-'
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
}
