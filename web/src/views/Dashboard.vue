<template>
  <div class="wm-page" v-loading="!loaded">
    <WmSectionCard dense>
      <WmToolbar>
        <span class="node-label">监控对象</span>
        <el-select v-model="nodeSel" style="width: 250px" aria-label="监控对象">
          <el-option label="本机（面板所在服务器）" value="local" />
          <el-option v-for="n in nodes" :key="n.node.id" :value="String(n.node.id)"
            :label="`${n.node.name}${n.online ? '' : '（离线）'}`" />
        </el-select>
        <template #note>本机指标每 3 秒采样；远程节点由 Agent 每 15 秒上报一次。</template>
      </WmToolbar>
    </WmSectionCard>

    <div class="wm-grid-4">
      <WmStatCard
        label="CPU 使用率"
        :value="fix(current.cpu)"
        unit="%"
        :tone="(current.cpu || 0) > 85 ? 'danger' : 'info'"
        :progress="Math.min(100, current.cpu || 0)"
        :loading="!loaded"
      >
        <template #icon><el-icon><Cpu /></el-icon></template>
      </WmStatCard>

      <WmStatCard
        label="内存"
        :value="fmtBytes(current.mem_used)"
        :tone="(current.mem_pct || 0) > 85 ? 'danger' : 'success'"
        :progress="Math.min(100, current.mem_pct || 0)"
        :loading="!loaded"
      >
        <template #icon><el-icon><Memo /></el-icon></template>
        <template #sub>共 {{ fmtBytes(current.mem_total) }} · 使用率 {{ fix(current.mem_pct) }}%</template>
      </WmStatCard>

      <WmStatCard
        label="1 分钟负载"
        :value="current.load1 != null ? current.load1.toFixed(2) : '-'"
        tone="warning"
        :loading="!loaded"
      >
        <template #icon><el-icon><TrendCharts /></el-icon></template>
        <template #sub>网络 ↓ {{ fmtBytes(current.recv) }}/s · ↑ {{ fmtBytes(current.sent) }}/s</template>
      </WmStatCard>

      <WmStatCard label="系统环境" :text="true" :value="[host.platform, host.platform_version].filter(Boolean).join(' ') || '-'" tone="neutral" :loading="!loaded">
        <template #icon><el-icon><Platform /></el-icon></template>
        <template #sub>
          <template v-if="remote">远程节点 · 最近上报 {{ fmtTime(current.time) }}</template>
          <template v-else>
            {{ host.hostname || '本机' }}<span v-if="host.kernel_version"> · 内核 {{ host.kernel_version }} ({{ host.kernel_arch }})</span><br />
            已运行 {{ fmtDuration(host.uptime) }}
          </template>
        </template>
      </WmStatCard>
    </div>

    <div class="wm-grid-2">
      <WmSectionCard title="CPU / 内存走势" description="实时曲线 · 百分比" quiet>
        <div ref="cpuEl" class="wm-chart" style="height: 272px"></div>
      </WmSectionCard>
      <WmSectionCard title="网络流量" description="下载 / 上传 每秒速率" quiet>
        <div ref="netEl" class="wm-chart" style="height: 272px"></div>
      </WmSectionCard>
    </div>

    <template v-if="!remote">
      <div class="wm-grid-2 wm-grid-2--wide-left">
        <WmSectionCard title="磁盘" description="挂载点用量" flush>
          <WmTable :data="disks" size="small" empty-title="未检测到磁盘信息">
            <el-table-column prop="mount" label="挂载点" min-width="110" />
            <el-table-column label="用量" min-width="150">
              <template #default="{ row }">
                <el-progress :percentage="Math.min(100, Math.round(row.used_pct))" :stroke-width="10"
                  :status="row.used_pct > 90 ? 'exception' : row.used_pct > 75 ? 'warning' : undefined" />
              </template>
            </el-table-column>
            <el-table-column label="已用/总量" width="150">
              <template #default="{ row }"><span class="wm-mono">{{ fmtBytes(row.used) }} / {{ fmtBytes(row.total) }}</span></template>
            </el-table-column>
          </WmTable>
        </WmSectionCard>
        <WmSectionCard title="CPU 占用 Top 进程" description="按当前占用排序" flush>
          <WmTable :data="procs" size="small" empty-title="暂无进程数据">
            <el-table-column prop="pid" label="PID" width="90"><template #default="{ row }"><span class="wm-mono">{{ row.pid }}</span></template></el-table-column>
            <el-table-column prop="name" label="进程" min-width="180" />
            <el-table-column label="CPU%" width="100">
              <template #default="{ row }"><span :class="{ hot: row.cpu > 60 }" class="wm-mono">{{ row.cpu.toFixed(1) }}</span></template>
            </el-table-column>
            <el-table-column label="内存%" width="100">
              <template #default="{ row }"><span class="wm-mono">{{ row.mem_pct.toFixed(1) }}</span></template>
            </el-table-column>
          </WmTable>
        </WmSectionCard>
      </div>
    </template>

    <WmNotice v-else tone="info" title="远程节点仅采集基础指标">
      当前展示 CPU / 内存 / 网络 / 负载；磁盘与进程信息请通过「节点管理 → 命令」或终端获取。
    </WmNotice>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch, nextTick } from 'vue'
import * as echarts from 'echarts'
import { Cpu, Memo, TrendCharts, Platform } from '@element-plus/icons-vue'
import api from '../api'
import { fmtBytes, fmtDuration, fmtTime, chartPalette } from '../utils'

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
    xAxis: {
      type: 'time' as const,
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: p.muted, fontSize: 10 },
      splitLine: { show: false }
    }
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
      {
        name: 'CPU %', type: 'line', showSymbol: false, smooth: true,
        lineStyle: { width: 2.2, color: p.accent }, itemStyle: { color: p.accent },
        areaStyle: { color: grad(p.accent, 0.26) },
        data: hist.map(([t, s]) => [t, +Number(s.cpu).toFixed(1)])
      },
      {
        name: '内存 %', type: 'line', showSymbol: false, smooth: true,
        lineStyle: { width: 2.2, color: p.success }, itemStyle: { color: p.success },
        areaStyle: { color: grad(p.success, 0.22) },
        data: hist.map(([t, s]) => [t, +Number(s.mem_pct).toFixed(1)])
      }
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
      {
        name: '下载', type: 'line', showSymbol: false, smooth: true,
        lineStyle: { width: 2.2, color: p.accent }, itemStyle: { color: p.accent },
        areaStyle: { color: grad(p.accent, 0.22) },
        data: hist.map(([t, s]) => [t, s.recv])
      },
      {
        name: '上传', type: 'line', showSymbol: false, smooth: true,
        lineStyle: { width: 2.2, color: p.warning }, itemStyle: { color: p.warning },
        areaStyle: { color: grad(p.warning, 0.2) },
        data: hist.map(([t, s]) => [t, s.sent])
      }
    ]
  } as any, true)
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
