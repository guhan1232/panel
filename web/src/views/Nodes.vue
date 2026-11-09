<template>
  <div class="wm-page">
    <WmSectionCard title="Agent 接入" description="通过 WebSocket 反连，被控端无需开放端口" marker>
      <WmToolbar>
        <a-input :value="agentCmd" readonly class="wm-agent-cmd" aria-label="Agent 接入命令" />
        <a-button @click="copyCmd">复制</a-button>
        <a-button v-if="auth.role === 'admin'" @click="regenToken" data-tone="warning">重置 Token</a-button>
        <a-button @click="load">刷新</a-button>
      </WmToolbar>
      <WmNotice tone="neutral">在目标服务器下载对应架构的 hanyuxin-agent 并运行（需面板防火墙放行面板端口）；接入后每 15 秒上报一次指标，断线自动重连。</WmNotice>
    </WmSectionCard>

    <WmSectionCard title="节点列表" description="在线状态依据最近心跳判定" flush>
      <WmToolbar>
        <a-button size="small" @click="load">刷新</a-button>
        <template #note>共 {{ nodes.length }} 个节点，其中 {{ nodes.filter((n) => n.online).length }} 个在线。</template>
      </WmToolbar>
      <WmTable :data="nodes" v-loading="loading" empty-title="还没有接入节点" empty-description="复制上方接入命令，在目标服务器启动 Agent 后即可在此看到节点。">
        <WmColumn label="节点" min-width="130"><template #default="{ row }"><b>{{ row.node.name }}</b></template></WmColumn>
        <WmColumn label="主机" min-width="140"><template #default="{ row }"><span class="wm-mono">{{ row.node.hostname }}</span></template></WmColumn>
        <WmColumn label="系统/架构" min-width="160"><template #default="{ row }">{{ row.node.os }} / {{ row.node.arch }}</template></WmColumn>
        <WmColumn label="IP" width="140"><template #default="{ row }"><span class="wm-mono">{{ row.node.ip }}</span></template></WmColumn>
        <WmColumn label="状态" width="100">
          <template #default="{ row }"><WmStatusPill :label="row.online ? '在线' : '离线'" :tone="row.online ? 'success' : 'muted'" :pulse="row.online" /></template>
        </WmColumn>
        <WmColumn label="最近心跳" width="170"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.node.last_seen) }}</span></template></WmColumn>
        <WmColumn label="Agent" width="90"><template #default="{ row }"><span class="wm-mono">{{ row.node.version }}</span></template></WmColumn>
        <WmColumn label="操作" width="230" fixed="right">
          <template #default="{ row }">
            <a-button size="small" @click="openMetrics(row)">指标</a-button>
            <a-button size="small" data-tone="success" :disabled="!row.online || auth.role === 'viewer'" @click="openExec(row)">命令</a-button>
            <a-button size="small" :disabled="!row.online || auth.role === 'viewer'" @click="openFiles(row)">文件</a-button>
            <a-button size="small" :disabled="!row.online || auth.role === 'viewer'" @click="openNodeTerm(row)">终端</a-button>
            <a-button size="small" data-tone="danger" v-if="auth.role === 'admin'" @click="del(row)">删除</a-button>
          </template>
        </WmColumn>
      </WmTable>
    </WmSectionCard>

    <a-drawer v-model:open="mDlg" width="62%" :title="`节点指标 - ${mNode?.node?.name || ''}`">
      <WmToolbar>
        <a-radio-group v-model:value="mRange" size="small" @change="loadMetrics">
          <a-radio-button value="hour">近 1 小时</a-radio-button>
          <a-radio-button value="month">近 30 天</a-radio-button>
        </a-radio-group>
        <a-button size="small" @click="loadMetrics">刷新</a-button>
        <template #note>{{ mRange === 'hour' ? '近 1 小时（15s 采样，保留 3 天）' : '近 30 天（小时聚合，长期保留）' }}</template>
      </WmToolbar>
      <WmSectionCard title="CPU / 内存" dense quiet class="wm-mb-4">
        <div ref="chartEl" class="wm-chart" style="height: 300px"></div>
      </WmSectionCard>
      <WmSectionCard v-show="mRange === 'hour'" title="网络流量" dense quiet>
        <div ref="netEl" class="wm-chart" style="height: 240px"></div>
      </WmSectionCard>
    </a-drawer>

    <a-drawer v-model:open="fDlg" width="62%" :title="`节点文件 - ${fNode?.name || ''}`">
      <WmToolbar>
        <a-input v-model:value="fPath" placeholder="绝对路径，如 /root" style="width: 420px" @press-enter="loadFiles" />
        <a-button size="small" @click="loadFiles">前往</a-button>
        <a-button size="small" @click="fUp()">上级</a-button>
        <a-upload :before-upload="uploadFile" :show-upload-list="false"><a-button size="small" type="primary">上传到当前目录</a-button></a-upload>
      </WmToolbar>
      <WmTable :data="files" v-loading="fLoading" size="small" max-height="480" empty-title="空目录">
        <WmColumn label="名称" min-width="240">
          <template #default="{ row }">
            <a v-if="row.is_dir" class="wm-mono" @click="enterDir(row)">{{ row.name }}/</a>
            <span v-else class="wm-mono">{{ row.name }}</span>
          </template>
        </WmColumn>
        <WmColumn label="大小" width="110"><template #default="{ row }">{{ row.is_dir ? '-' : fmtSize(row.size) }}</template></WmColumn>
        <WmColumn label="权限" width="120"><template #default="{ row }"><span class="wm-mono">{{ row.mode }}</span></template></WmColumn>
        <WmColumn label="修改时间" width="160"><template #default="{ row }"><span class="wm-mono">{{ row.mod_time }}</span></template></WmColumn>
        <WmColumn label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <a-button v-if="!row.is_dir" size="small" @click="downloadFile(row)">下载</a-button>
            <a-button size="small" data-tone="danger" @click="deleteFile(row)">删除</a-button>
          </template>
        </WmColumn>
      </WmTable>
      <WmNotice tone="warning" class="wm-mt-4">远程文件操作经 Agent 通道执行并审计；删除为递归删除，请谨慎。</WmNotice>
    </a-drawer>

    <a-drawer v-model:open="tDlg" width="62%" :title="`节点终端 - ${tNode?.name || ''}`" @close="closeNodeTerm">
      <div ref="ntBox" style="height: 460px; background: #14181f"></div>
    </a-drawer>

    <a-modal v-model:open="eDlg" :title="`远程命令 - ${eNode?.node?.name || ''}`" width="720px">
      <a-textarea v-model:value="eScript"  :rows="6" placeholder="bash 命令，如：df -h" />
      <WmNotice tone="warning" class="wm-mt-2 wm-mb-4">超时 {{ eTimeout }}s；命令经审计记录，请确认执行内容。</WmNotice>
      <WmCodeBlock v-if="eResult" :content="eResult" label="remote exec output" />
      <template #footer>
        <a-button @click="eDlg = false">关闭</a-button>
        <a-button type="primary" :loading="eRunning" @click="runExec">执行</a-button>
      </template>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import * as echarts from 'echarts'
import { ElMessage, ElMessageBox } from '../antd-compat'
import api from '../api'
import { fmtBytes, fmtTime, chartPalette } from '../utils'
import { useAuth } from '../stores/auth'

const auth = useAuth()
const nodes = ref<any[]>([])
const loading = ref(false)
const token = ref('')
const mDlg = ref(false)
const mRange = ref<'hour' | 'month'>('hour')
const eDlg = ref(false)
const mNode = ref<any>(null)
const eNode = ref<any>(null)
const eScript = ref('')
const eTimeout = ref(120)
const eResult = ref('')
const eRunning = ref(false)
const chartEl = ref<HTMLElement>()
const netEl = ref<HTMLElement>()
let cpuChart: echarts.ECharts | null = null
let netChart: echarts.ECharts | null = null

const agentCmd = computed(() =>
  `./hanyuxin-agent -s ${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host} -t ${token.value || '<token>'} -name <节点名>`
)

function fade(hex: string, alpha: number) {
  const h = hex.replace('#', '')
  const full = h.length === 3 ? h.split('').map((c) => c + c).join('') : h
  const n = parseInt(full, 16)
  return `rgba(${(n >> 16) & 255}, ${(n >> 8) & 255}, ${n & 255}, ${alpha})`
}
function grad(color: string, alpha: number) {
  return new echarts.graphic.LinearGradient(0, 0, 0, 1, [
    { offset: 0, color: fade(color, alpha) },
    { offset: 1, color: fade(color, 0.01) }
  ])
}

async function load() {
  loading.value = true
  try {
    nodes.value = (await api.get('/nodes')) as any
    if (auth.role === 'admin') {
      try {
        token.value = ((await api.get('/nodes/token')) as any).token
      } catch {
        /* 非 admin 忽略 */
      }
    }
  } finally {
    loading.value = false
  }
}

async function copyCmd() {
  try {
    await navigator.clipboard.writeText(agentCmd.value)
    ElMessage.success('已复制')
  } catch {
    ElMessage.warning(agentCmd.value)
  }
}

async function regenToken() {
  try {
    await ElMessageBox.confirm('重置后已有 Agent 将无法重新连接（在连会话保留），需用新 token 重启 Agent，继续？', '重置 Token', { type: 'warning' })
  } catch {
    return
  }
  token.value = ((await api.post('/nodes/token/regenerate')) as any).token
  ElMessage.success('已重置')
}

function openMetrics(row: any) {
  mNode.value = row
  mDlg.value = true
  nextTick(() => {
    if (!cpuChart && chartEl.value) cpuChart = echarts.init(chartEl.value)
    if (!netChart && netEl.value) netChart = echarts.init(netEl.value)
    loadMetrics()
  })
}

async function loadMetrics() {
  if (!mNode.value) return
  let hist: any[]
  if (mRange.value === 'month') {
    const rows: any[] = (await api.get(`/nodes/${mNode.value.node.id}/metrics/hourly?days=30`)) as any
    hist = rows.map((r: any) => [new Date(r.hour).getTime(), r])
  } else {
    const rows: any[] = (await api.get(`/nodes/${mNode.value.node.id}/metrics?limit=240`)) as any
    hist = rows.map((r) => [new Date(r.time).getTime(), r])
  }
  const p = chartPalette()
  const base = {
    tooltip: {
      trigger: 'axis' as const,
      backgroundColor: p.surface,
      borderColor: p.line,
      borderWidth: 1,
      textStyle: { color: p.ink, fontSize: 12 },
      axisPointer: { lineStyle: { color: p.line } }
    },
    legend: { top: 0, right: 0, itemWidth: 12, itemHeight: 8, textStyle: { color: p.muted, fontSize: 11 } },
    xAxis: { type: 'time' as const, axisLine: { show: false }, axisTick: { show: false }, axisLabel: { color: p.muted, fontSize: 10 } }
  }

  cpuChart?.setOption({
    ...base,
    legend: { ...base.legend, data: ['CPU %', '内存 %'] },
    grid: { left: 44, right: 12, top: 30, bottom: 24 },
    yAxis: {
      type: 'value' as const, max: 100,
      axisLabel: { color: p.muted, fontSize: 10 },
      splitLine: { lineStyle: { color: p.line, type: 'dashed' as const } }
    },
    series: [
      { name: 'CPU %', type: 'line', showSymbol: false, smooth: true, lineStyle: { width: 2.2, color: p.accent }, itemStyle: { color: p.accent }, areaStyle: { color: grad(p.accent, 0.24) }, data: hist.map(([t, r]: any) => [t, +(mRange.value === 'month' ? r.avg_cpu : r.cpu).toFixed(1)]) },
      { name: '内存 %', type: 'line', showSymbol: false, smooth: true, lineStyle: { width: 2.2, color: p.success }, itemStyle: { color: p.success }, areaStyle: { color: grad(p.success, 0.2) }, data: hist.map(([t, r]: any) => [t, +(mRange.value === 'month' ? r.avg_mem : r.mem_pct).toFixed(1)]) }
    ]
  } as any, true)
  netChart?.setOption({
    ...base,
    tooltip: { ...base.tooltip, valueFormatter: (v: number) => fmtBytes(v) + '/s' },
    legend: { ...base.legend, data: ['下载', '上传'] },
    grid: { left: 64, right: 12, top: 30, bottom: 24 },
    yAxis: {
      type: 'value' as const,
      axisLabel: { color: p.muted, fontSize: 10, formatter: (v: number) => fmtBytes(v) },
      splitLine: { lineStyle: { color: p.line, type: 'dashed' as const } }
    },
    series: [
      { name: '下载', ...(mRange.value === 'month' ? { data: [] } : {}), type: 'line', showSymbol: false, smooth: true, lineStyle: { width: 2.2, color: p.accent }, itemStyle: { color: p.accent }, areaStyle: { color: grad(p.accent, 0.2) }, data: hist.map(([t, r]) => [t, r.recv]) },
      { name: '上传', type: 'line', showSymbol: false, smooth: true, lineStyle: { width: 2.2, color: p.warning }, itemStyle: { color: p.warning }, areaStyle: { color: grad(p.warning, 0.18) }, data: hist.map(([t, r]) => [t, r.sent]) }
    ]
  } as any, true)
}

function openExec(row: any) {
  eNode.value = row
  eScript.value = ''
  eResult.value = ''
  eDlg.value = true
}

async function runExec() {
  if (!eScript.value.trim()) {
    ElMessage.warning('请输入命令')
    return
  }
  eRunning.value = true
  eResult.value = ''
  try {
    const d: any = await api.post(`/nodes/${eNode.value.node.id}/exec`, { script: eScript.value, timeout_sec: eTimeout.value }, { timeout: 600000 })
    eResult.value = (d.err ? `执行错误: ${d.err}\n` : '') + (d.output || '（无输出）') + `\n[exit ${d.exit}]`
  } finally {
    eRunning.value = false
  }
}

async function del(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除节点「${row.node.name}」？其历史指标将一并删除`, '删除节点', { type: 'warning' })
  } catch {
    return
  }
  await api.delete(`/nodes/${row.node.id}`)
  load()
}

onBeforeUnmount(() => {
  cpuChart?.dispose()
  netChart?.dispose()
})

onMounted(load)

const fDlg = ref(false)
const fNode = ref<any>(null)
const fPath = ref('/root')
const files = ref<any[]>([])
const fLoading = ref(false)
const tDlg = ref(false)
const tNode = ref<any>(null)
let ntTerm: any = null
let ntWS: WebSocket | null = null
let ntBox: any = null
let ntFit: any = null

function fmtSize(x: number) {
  if (x >= 1048576) return (x / 1048576).toFixed(1) + ' MB'
  if (x >= 1024) return (x / 1024).toFixed(1) + ' KB'
  return x + ' B'
}

function openFiles(row: any) {
  fNode.value = { id: row.node.id, name: row.node.name }
  fPath.value = '/root'
  fDlg.value = true
  loadFiles()
}

async function loadFiles() {
  fLoading.value = true
  try {
    const d: any = await api.get(`/nodes/${fNode.value.id}/files`, { params: { path: fPath.value } })
    files.value = d.entries || []
    fPath.value = d.path
  } catch (e: any) {
    ElMessage.error(e?.msg || '目录读取失败')
  } finally {
    fLoading.value = false
  }
}

function enterDir(row: any) {
  fPath.value = (fPath.value.replace(/\/+$/, '')) + '/' + row.name
  loadFiles()
}

function fUp() {
  fPath.value = fPath.value.replace(/\/+$/, '').split('/').slice(0, -1).join('/') || '/'
  loadFiles()
}

async function downloadFile(row: any) {
  const d: any = await api.get(`/nodes/${fNode.value.id}/file`, { params: { path: fPath.value.replace(/\/+$/, '') + '/' + row.name } })
  const bin = atob(d.data)
  const arr = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) arr[i] = bin.charCodeAt(i)
  const url = URL.createObjectURL(new Blob([arr]))
  const a = document.createElement('a')
  a.href = url
  a.download = row.name
  a.click()
  URL.revokeObjectURL(url)
}

async function deleteFile(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除远程文件「${row.name}」？（目录递归删除）`, '删除', { type: 'warning' })
  } catch { return }
  await api.delete(`/nodes/${fNode.value.id}/file`, { params: { path: fPath.value.replace(/\/+$/, '') + '/' + row.name, force: 1 } })
  ElMessage.success('已删除')
  loadFiles()
}

async function uploadFile(file: File) {
  const reader = new FileReader()
  reader.onload = async () => {
    const b64 = btoa(new Uint8Array(reader.result as ArrayBuffer).reduce((s, b) => s + String.fromCharCode(b), ''))
    const name = fPath.value.replace(/\/+$/, '') + '/' + file.name
    await api.post(`/nodes/${fNode.value.id}/file`, { path: name, data: b64 })
    ElMessage.success('已上传 ' + file.name)
    loadFiles()
  }
  reader.readAsArrayBuffer(file)
  return false
}

function openNodeTerm(row: any) {
  tNode.value = { id: row.node.id, name: row.node.name }
  tDlg.value = true
  setTimeout(() => {
    connectNodeTerm()
  }, 300)
}

async function connectNodeTerm() {
  if (!ntBox.value) {
    await nextTick()
  }
  const XTerm = await import('@xterm/xterm').then((m) => m.Terminal)
  const XFit = await import('@xterm/addon-fit').then((m) => m.FitAddon)
  await import('@xterm/xterm/css/xterm.css').catch(() => {})
  ntTerm = new XTerm({ cursorBlink: true, fontSize: 13, theme: { background: '#14181f' } })
  ntFit = new XFit()
  ntTerm.loadAddon(ntFit)
  if (ntBox.value) {
    ntTerm.open(ntBox.value)
    ntFit.fit()
  }
  ntTerm.onData((d: string) => {
    if (ntWS && ntWS.readyState === 1) ntWS.send(d)
  })
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  ntWS = new WebSocket(`${proto}://${location.host}/api/ws/nodeterm/${tNode.value.id}`)
  ntWS.onmessage = (ev: MessageEvent) => {
    ntTerm?.write(ev.data)
  }
  ntTerm.writeln('\x1b[36m寒语馨面板 · 节点终端\x1b[0m  连接中...')
}

function closeNodeTerm() {
  ntWS?.close()
  ntWS = null
  ntTerm?.dispose()
  ntTerm = null
}
</script>
