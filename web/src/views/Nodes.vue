<template>
  <div class="wm-page">
    <WmSectionCard title="Agent 接入" description="通过 WebSocket 反连，被控端无需开放端口" marker>
      <WmToolbar>
        <a-input :value="agentCmd" readonly class="wm-agent-cmd" aria-label="Agent 接入命令" />
        <a-button @click="copyCmd">复制</a-button>
        <a-button v-if="auth.role === 'admin'" @click="regenToken" data-tone="warning">重置 Token</a-button>
        <a-button @click="load">刷新</a-button>
      </WmToolbar>
      <WmNotice tone="neutral">在目标服务器下载对应架构的 webmblx-agent 并运行（需面板防火墙放行面板端口）；接入后每 15 秒上报一次指标，断线自动重连。</WmNotice>
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
            <a-button size="small" data-tone="danger" v-if="auth.role === 'admin'" @click="del(row)">删除</a-button>
          </template>
        </WmColumn>
      </WmTable>
    </WmSectionCard>

    <a-drawer v-model:open="mDlg" width="62%" :title="`节点指标 - ${mNode?.node?.name || ''}`">
      <WmToolbar>
        <a-button size="small" @click="loadMetrics">刷新</a-button>
        <template #note>近 1 小时（15s 采样，保留 3 天）</template>
      </WmToolbar>
      <WmSectionCard title="CPU / 内存" dense quiet class="wm-mb-4">
        <div ref="chartEl" class="wm-chart" style="height: 300px"></div>
      </WmSectionCard>
      <WmSectionCard title="网络流量" dense quiet>
        <div ref="netEl" class="wm-chart" style="height: 240px"></div>
      </WmSectionCard>
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
  `./webmblx-agent -s ${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host} -t ${token.value || '<token>'} -name <节点名>`
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
  const rows: any[] = (await api.get(`/nodes/${mNode.value.node.id}/metrics?limit=240`)) as any
  const hist = rows.map((r) => [new Date(r.time).getTime(), r])
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
      { name: 'CPU %', type: 'line', showSymbol: false, smooth: true, lineStyle: { width: 2.2, color: p.accent }, itemStyle: { color: p.accent }, areaStyle: { color: grad(p.accent, 0.24) }, data: hist.map(([t, r]) => [t, +r.cpu.toFixed(1)]) },
      { name: '内存 %', type: 'line', showSymbol: false, smooth: true, lineStyle: { width: 2.2, color: p.success }, itemStyle: { color: p.success }, areaStyle: { color: grad(p.success, 0.2) }, data: hist.map(([t, r]) => [t, +r.mem_pct.toFixed(1)]) }
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
      { name: '下载', type: 'line', showSymbol: false, smooth: true, lineStyle: { width: 2.2, color: p.accent }, itemStyle: { color: p.accent }, areaStyle: { color: grad(p.accent, 0.2) }, data: hist.map(([t, r]) => [t, r.recv]) },
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
</script>
