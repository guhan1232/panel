<template>
  <div>
    <el-card class="page-card" header="Agent 接入">
      <div class="toolbar">
        <el-input :model-value="agentCmd" readonly style="max-width: 640px" />
        <el-button @click="copyCmd">复制</el-button>
        <el-button v-if="auth.role === 'admin'" @click="regenToken" type="warning" plain>重置 Token</el-button>
        <el-button @click="load">刷新</el-button>
      </div>
      <div class="hint">
        在目标服务器上下载对应架构的 webmblx-agent 并运行（需面板防火墙放行面板端口）；接入后每 15 秒上报一次指标，断线自动重连。
      </div>
    </el-card>

    <el-card class="page-card" header="节点列表">
      <el-table :data="nodes" v-loading="loading">
        <el-table-column label="节点" min-width="120"><template #default="{ row }"><b>{{ row.node.name }}</b></template></el-table-column>
        <el-table-column label="主机" min-width="140"><template #default="{ row }">{{ row.node.hostname }}</template></el-table-column>
        <el-table-column label="系统/架构" min-width="160"><template #default="{ row }">{{ row.node.os }} / {{ row.node.arch }}</template></el-table-column>
        <el-table-column label="IP" width="130"><template #default="{ row }">{{ row.node.ip }}</template></el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag size="small" :type="row.online ? 'success' : 'info'">{{ row.online ? '在线' : '离线' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="最近心跳" width="165"><template #default="{ row }">{{ fmtTime(row.node.last_seen) }}</template></el-table-column>
        <el-table-column label="Agent" width="80"><template #default="{ row }">{{ row.node.version }}</template></el-table-column>
        <el-table-column label="操作" width="230" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openMetrics(row)">指标</el-button>
            <el-button size="small" type="success" :disabled="!row.online || auth.role === 'viewer'" @click="openExec(row)">命令</el-button>
            <el-button size="small" type="danger" v-if="auth.role === 'admin'" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-drawer v-model="mDlg" size="62%" :title="`节点指标 - ${mNode?.node?.name || ''}`">
      <div class="toolbar">
        <el-button size="small" @click="loadMetrics">刷新</el-button>
        <span class="hint">近 1 小时（15s 采样，保留 3 天）</span>
      </div>
      <div ref="chartEl" style="height: 300px"></div>
      <div ref="netEl" style="height: 240px; margin-top: 8px"></div>
    </el-drawer>

    <el-dialog v-model="eDlg" :title="`远程命令 - ${eNode?.node?.name || ''}`" width="720px">
      <el-input v-model="eScript" type="textarea" :rows="6" placeholder="bash 命令，如：df -h" />
      <div class="hint" style="margin: 8px 0">超时 {{ eTimeout }}s；命令经审计记录</div>
      <pre class="pre-output" v-if="eResult">{{ eResult }}</pre>
      <template #footer>
        <el-button @click="eDlg = false">关闭</el-button>
        <el-button type="primary" :loading="eRunning" @click="runExec">执行</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import * as echarts from 'echarts'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'
import { fmtBytes, fmtTime } from '../utils'
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

const splitLine = { lineStyle: { color: 'rgba(128,140,160,0.18)' } }
function grad(c1: string, c2: string) {
  return new echarts.graphic.LinearGradient(0, 0, 0, 1, [
    { offset: 0, color: c1 },
    { offset: 1, color: c2 }
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
  cpuChart?.setOption({
    tooltip: { trigger: 'axis' },
    legend: { data: ['CPU %', '内存 %'], top: 0, right: 0 },
    grid: { left: 45, right: 12, top: 32, bottom: 26 },
    xAxis: { type: 'time', axisLine: { show: false } },
    yAxis: { type: 'value', max: 100, splitLine },
    series: [
      { name: 'CPU %', type: 'line', showSymbol: false, smooth: true, lineStyle: { width: 2, color: '#3d7fff' }, areaStyle: { color: grad('rgba(61,127,255,0.25)', 'rgba(61,127,255,0.01)') }, data: hist.map(([t, r]) => [t, +r.cpu.toFixed(1)]) },
      { name: '内存 %', type: 'line', showSymbol: false, smooth: true, lineStyle: { width: 2, color: '#2eb86a' }, areaStyle: { color: grad('rgba(46,184,106,0.22)', 'rgba(46,184,106,0.01)') }, data: hist.map(([t, r]) => [t, +r.mem_pct.toFixed(1)]) }
    ]
  }, true)
  netChart?.setOption({
    tooltip: { trigger: 'axis', valueFormatter: (v: number) => fmtBytes(v) + '/s' },
    legend: { data: ['下载', '上传'], top: 0, right: 0 },
    grid: { left: 64, right: 12, top: 32, bottom: 26 },
    xAxis: { type: 'time', axisLine: { show: false } },
    yAxis: { type: 'value', splitLine, axisLabel: { formatter: (v: number) => fmtBytes(v) } },
    series: [
      { name: '下载', type: 'line', showSymbol: false, smooth: true, lineStyle: { width: 2, color: '#3d7fff' }, areaStyle: { color: grad('rgba(61,127,255,0.2)', 'rgba(61,127,255,0.01)') }, data: hist.map(([t, r]) => [t, r.recv]) },
      { name: '上传', type: 'line', showSymbol: false, smooth: true, lineStyle: { width: 2, color: '#f59e0b' }, areaStyle: { color: grad('rgba(245,158,11,0.2)', 'rgba(245,158,11,0.01)') }, data: hist.map(([t, r]) => [t, r.sent]) }
    ]
  }, true)
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

<style scoped>
.hint { color: #999; font-size: 12px; }
</style>
