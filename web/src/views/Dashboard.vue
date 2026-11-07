<template>
  <div v-loading="!loaded">
    <div class="toolbar" style="justify-content: flex-end">
      <span class="node-label">监控对象：</span>
      <el-select v-model="nodeSel" style="width: 220px">
        <el-option label="本机（面板所在服务器）" value="local" />
        <el-option v-for="n in nodes" :key="n.node.id" :value="String(n.node.id)"
          :label="`${n.node.name}${n.online ? '' : '（离线）'}`" />
      </el-select>
    </div>

    <el-row :gutter="16">
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card class="page-card stat-card">
          <div class="stat-top">
            <div>
              <div class="stat-label">CPU 使用率</div>
              <div class="stat-value" :class="{ hot: (current.cpu || 0) > 85 }">
                {{ fix(current.cpu) }}<span class="stat-unit">%</span>
              </div>
            </div>
            <div class="stat-icon" :class="(current.cpu || 0) > 85 ? 'hot' : 'blue'"><el-icon :size="22"><Cpu /></el-icon></div>
          </div>
          <el-progress :percentage="Math.min(100, current.cpu || 0)" :stroke-width="6" :show-text="false" />
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card class="page-card stat-card">
          <div class="stat-top">
            <div>
              <div class="stat-label">内存</div>
              <div class="stat-value" :class="{ hot: (current.mem_pct || 0) > 85 }">
                {{ fmtBytes(current.mem_used) }}
              </div>
            </div>
            <div class="stat-icon" :class="(current.mem_pct || 0) > 85 ? 'hot' : 'green'"><el-icon :size="22"><Memo /></el-icon></div>
          </div>
          <el-progress :percentage="Math.min(100, current.mem_pct || 0)" :stroke-width="6" :show-text="false" />
          <div class="stat-sub">共 {{ fmtBytes(current.mem_total) }} · 使用率 {{ fix(current.mem_pct) }}%</div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card class="page-card stat-card">
          <div class="stat-top">
            <div>
              <div class="stat-label">1 分钟负载</div>
              <div class="stat-value">{{ current.load1 != null ? current.load1.toFixed(2) : '-' }}</div>
            </div>
            <div class="stat-icon orange"><el-icon :size="22"><TrendCharts /></el-icon></div>
          </div>
          <div class="stat-sub">网络 ↓ {{ fmtBytes(current.recv) }}/s · ↑ {{ fmtBytes(current.sent) }}/s</div>
        </el-card>
      </el-col>
      <el-col :xs="24" :sm="12" :lg="6">
        <el-card class="page-card stat-card">
          <div class="stat-top">
            <div>
              <div class="stat-label">{{ host.hostname || '系统' }}</div>
              <div class="stat-value" style="font-size: 16px; line-height: 1.6">
                {{ host.platform }} {{ host.platform_version }}
              </div>
            </div>
            <div class="stat-icon slate"><el-icon :size="22"><Platform /></el-icon></div>
          </div>
          <div class="stat-sub">
            <template v-if="remote">远程节点 · 最近上报 {{ fmtTime(current.time) }}</template>
            <template v-else>
              <template v-if="host.kernel_version">内核 {{ host.kernel_version }} ({{ host.kernel_arch }})<br /></template>
              已运行 {{ fmtDuration(host.uptime) }}
            </template>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="16">
      <el-col :lg="14" :xs="24">
        <el-card class="page-card" header="CPU / 内存走势">
          <div ref="cpuEl" style="height: 270px"></div>
        </el-card>
      </el-col>
      <el-col :lg="10" :xs="24">
        <el-card class="page-card" header="网络流量">
          <div ref="netEl" style="height: 270px"></div>
        </el-card>
      </el-col>
    </el-row>

    <template v-if="!remote">
      <el-row :gutter="16">
        <el-col :lg="10" :xs="24">
          <el-card class="page-card" header="磁盘">
            <el-table :data="disks" size="small">
              <el-table-column prop="mount" label="挂载点" min-width="110" />
              <el-table-column label="用量" min-width="150">
                <template #default="{ row }">
                  <el-progress :percentage="Math.min(100, Math.round(row.used_pct))" :stroke-width="10"
                    :status="row.used_pct > 90 ? 'exception' : row.used_pct > 75 ? 'warning' : undefined" />
                </template>
              </el-table-column>
              <el-table-column label="已用/总量" width="140">
                <template #default="{ row }">{{ fmtBytes(row.used) }} / {{ fmtBytes(row.total) }}</template>
              </el-table-column>
            </el-table>
          </el-card>
        </el-col>
        <el-col :lg="14" :xs="24">
          <el-card class="page-card" header="CPU 占用 Top 进程">
            <el-table :data="procs" size="small">
              <el-table-column prop="pid" label="PID" width="90" />
              <el-table-column prop="name" label="进程" min-width="180" />
              <el-table-column label="CPU%" width="100">
                <template #default="{ row }"><span :class="{ hot: row.cpu > 60 }">{{ row.cpu.toFixed(1) }}</span></template>
              </el-table-column>
              <el-table-column label="内存%" width="100"><template #default="{ row }">{{ row.mem_pct.toFixed(1) }}</template></el-table-column>
            </el-table>
          </el-card>
        </el-col>
      </el-row>
    </template>
    <el-alert v-else type="info" :closable="false" title="远程节点当前仅采集 CPU/内存/网络/负载基础指标，磁盘与进程信息请通过「命令」或终端获取。" />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch, nextTick } from 'vue'
import * as echarts from 'echarts'
import { Cpu, Memo, TrendCharts, Platform } from '@element-plus/icons-vue'
import api from '../api'
import { fmtBytes, fmtDuration, fmtTime } from '../utils'

const current = ref<any>({})
const host = ref<any>({})
const disks = ref<any[]>([])
const procs = ref<any[]>([])
const loaded = ref(false)
const remote = ref(false)
const nodes = ref<any[]>([])
const nodeSel = ref('local')

const cpuEl = ref<HTMLElement>()
const netEl = ref<HTMLElement>()
let cpuChart: echarts.ECharts | null = null
let netChart: echarts.ECharts | null = null
let timer: number | null = null

function fix(v?: number) { return v == null ? '-' : v.toFixed(1) }

function grad(c1: string, c2: string) {
  return new echarts.graphic.LinearGradient(0, 0, 0, 1, [
    { offset: 0, color: c1 },
    { offset: 1, color: c2 }
  ])
}

const splitLine = { lineStyle: { color: 'rgba(128,140,160,0.18)' } }

async function refresh() {
  let hist: any[] = []
  if (nodeSel.value === 'local') {
    const d: any = await api.get('/dashboard/summary')
    current.value = d.current || {}
    host.value = d.host || {}
    disks.value = d.disks || []
    procs.value = d.procs || []
    remote.value = false
    hist = (d.history || []).map((s: any) => [s.time * 1000, s])
  } else {
    const rows: any[] = (await api.get(`/nodes/${nodeSel.value}/metrics?limit=120`)) as any
    hist = rows.map((r) => [new Date(r.time).getTime(), r])
    const n: any = nodes.value.find((x) => String(x.node.id) === nodeSel.value)
    host.value = {
      hostname: n?.node.name || '远程节点',
      platform: n?.node.os || '',
      platform_version: n?.node.arch || '',
      kernel_version: ''
    }
    if (rows.length) {
      const last = rows[rows.length - 1]
      current.value = {
        cpu: last.cpu, mem_pct: last.mem_pct, mem_used: last.mem_used,
        mem_total: last.mem_total, recv: last.recv, sent: last.sent,
        load1: last.load1, time: last.time
      }
    } else {
      current.value = {}
    }
    disks.value = []
    procs.value = []
    remote.value = true
  }
  loaded.value = true
  await nextTick()
  updateCharts(hist)
}

function updateCharts(hist: any[]) {
  if (!cpuChart && cpuEl.value) cpuChart = echarts.init(cpuEl.value)
  if (!netChart && netEl.value) netChart = echarts.init(netEl.value)
  cpuChart?.setOption({
    tooltip: { trigger: 'axis' },
    legend: { data: ['CPU %', '内存 %'], top: 0, right: 0, itemWidth: 14 },
    grid: { left: 45, right: 12, top: 34, bottom: 26 },
    xAxis: { type: 'time', axisLine: { show: false } },
    yAxis: { type: 'value', max: 100, splitLine },
    series: [
      {
        name: 'CPU %', type: 'line', showSymbol: false, smooth: true,
        lineStyle: { width: 2.5, color: '#3d7fff' }, itemStyle: { color: '#3d7fff' },
        areaStyle: { color: grad('rgba(61,127,255,0.28)', 'rgba(61,127,255,0.01)') },
        data: hist.map(([t, s]) => [t, +Number(s.cpu).toFixed(1)])
      },
      {
        name: '内存 %', type: 'line', showSymbol: false, smooth: true,
        lineStyle: { width: 2.5, color: '#2eb86a' }, itemStyle: { color: '#2eb86a' },
        areaStyle: { color: grad('rgba(46,184,106,0.24)', 'rgba(46,184,106,0.01)') },
        data: hist.map(([t, s]) => [t, +Number(s.mem_pct).toFixed(1)])
      }
    ]
  })
  netChart?.setOption({
    tooltip: { trigger: 'axis', valueFormatter: (v: number) => fmtBytes(v) + '/s' },
    legend: { data: ['下载', '上传'], top: 0, right: 0, itemWidth: 14 },
    grid: { left: 64, right: 12, top: 34, bottom: 26 },
    xAxis: { type: 'time', axisLine: { show: false } },
    yAxis: { type: 'value', splitLine, axisLabel: { formatter: (v: number) => fmtBytes(v) } },
    series: [
      {
        name: '下载', type: 'line', showSymbol: false, smooth: true,
        lineStyle: { width: 2.5, color: '#3d7fff' }, itemStyle: { color: '#3d7fff' },
        areaStyle: { color: grad('rgba(61,127,255,0.24)', 'rgba(61,127,255,0.01)') },
        data: hist.map(([t, s]) => [t, s.recv])
      },
      {
        name: '上传', type: 'line', showSymbol: false, smooth: true,
        lineStyle: { width: 2.5, color: '#f59e0b' }, itemStyle: { color: '#f59e0b' },
        areaStyle: { color: grad('rgba(245,158,11,0.22)', 'rgba(245,158,11,0.01)') },
        data: hist.map(([t, s]) => [t, s.sent])
      }
    ]
  })
}

async function loadNodes() {
  try {
    nodes.value = (await api.get('/nodes')) as any
  } catch {
    nodes.value = []
  }
}

function onResize() {
  cpuChart?.resize()
  netChart?.resize()
}

onMounted(() => {
  loadNodes()
  refresh()
  timer = window.setInterval(refresh, 3000)
  window.addEventListener('resize', onResize)
})

watch(nodeSel, () => refresh())

onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
  window.removeEventListener('resize', onResize)
  cpuChart?.dispose()
  netChart?.dispose()
})
</script>

<style scoped>
.hot { color: var(--el-color-danger); }
.node-label { color: #8492a6; font-size: 13px; }
</style>
