<template>
  <div v-loading="!loaded">
    <el-row :gutter="16">
      <el-col :span="6">
        <el-card class="page-card"><template #header>CPU 使用率</template>
          <div class="metric" :class="warn(current.cpu)">{{ fix(current.cpu) }}%</div>
          <el-progress :percentage="Math.min(100, current.cpu || 0)" :stroke-width="8" :show-text="false" />
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="page-card"><template #header>内存使用</template>
          <div class="metric">{{ fmtBytes(current.mem_used) }} <span class="sub">/ {{ fmtBytes(current.mem_total) }}</span></div>
          <el-progress :percentage="Math.min(100, current.mem_pct || 0)" :stroke-width="8" :show-text="false" />
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="page-card"><template #header>负载 (1m)</template>
          <div class="metric">{{ current.load1 != null ? current.load1.toFixed(2) : '-' }}</div>
          <div class="sub">网络 ↓ {{ fmtBytes(current.recv) }}/s ↑ {{ fmtBytes(current.sent) }}/s</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card class="page-card"><template #header>系统信息</template>
          <div class="sub">
            <div>{{ host.hostname }} · {{ host.platform }} {{ host.platform_version }}</div>
            <div>内核 {{ host.kernel_version }} ({{ host.kernel_arch }})</div>
            <div>已运行 {{ fmtDuration(host.uptime) }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <el-card class="page-card" header="CPU / 内存（近 6 分钟）">
      <div ref="cpuEl" style="height: 260px"></div>
    </el-card>
    <el-row :gutter="16">
      <el-col :span="12">
        <el-card class="page-card" header="网络流量">
          <div ref="netEl" style="height: 240px"></div>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card class="page-card" header="磁盘">
          <el-table :data="disks" size="small">
            <el-table-column prop="mount" label="挂载点" min-width="120" />
            <el-table-column label="用量" min-width="180">
              <template #default="{ row }">
                <el-progress :percentage="Math.min(100, Math.round(row.used_pct))" :stroke-width="10"
                  :status="row.used_pct > 90 ? 'exception' : row.used_pct > 75 ? 'warning' : undefined" />
              </template>
            </el-table-column>
            <el-table-column label="已用/总量" width="150">
              <template #default="{ row }">{{ fmtBytes(row.used) }} / {{ fmtBytes(row.total) }}</template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>
    </el-row>

    <el-card class="page-card" header="CPU 占用 Top 进程">
      <el-table :data="procs" size="small">
        <el-table-column prop="pid" label="PID" width="90" />
        <el-table-column prop="name" label="进程" min-width="180" />
        <el-table-column label="CPU%" width="100"><template #default="{ row }">{{ row.cpu.toFixed(1) }}</template></el-table-column>
        <el-table-column label="内存%" width="100"><template #default="{ row }">{{ row.mem_pct.toFixed(1) }}</template></el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, reactive, ref, nextTick } from 'vue'
import * as echarts from 'echarts'
import api from '../api'
import { fmtBytes, fmtDuration } from '../utils'

const current = ref<any>({})
const host = ref<any>({})
const disks = ref<any[]>([])
const procs = ref<any[]>([])
const loaded = ref(false)

const cpuEl = ref<HTMLElement>()
const netEl = ref<HTMLElement>()
let cpuChart: echarts.ECharts | null = null
let netChart: echarts.ECharts | null = null
let timer: number | null = null

function warn(v?: number) { return v != null && v > 85 ? 'hot' : '' }
function fix(v?: number) { return v == null ? '-' : v.toFixed(1) }

async function refresh() {
  const d: any = await api.get('/dashboard/summary')
  current.value = d.current || {}
  host.value = d.host || {}
  disks.value = d.disks || []
  procs.value = d.procs || []
  loaded.value = true

  const hist = (d.history || []).map((s: any) => [s.time * 1000, s])
  await nextTick()
  if (!cpuChart && cpuEl.value) cpuChart = echarts.init(cpuEl.value)
  if (!netChart && netEl.value) netChart = echarts.init(netEl.value)
  cpuChart?.setOption({
    tooltip: { trigger: 'axis' },
    legend: { data: ['CPU %', '内存 %'] },
    grid: { left: 45, right: 16, top: 32, bottom: 26 },
    xAxis: { type: 'time' },
    yAxis: { type: 'value', max: 100 },
    series: [
      { name: 'CPU %', type: 'line', showSymbol: false, smooth: true, areaStyle: { opacity: 0.12 }, data: hist.map(([t, s]) => [t, +s.cpu.toFixed(1)]) },
      { name: '内存 %', type: 'line', showSymbol: false, smooth: true, areaStyle: { opacity: 0.12 }, data: hist.map(([t, s]) => [t, +s.mem_pct.toFixed(1)]) }
    ]
  })
  netChart?.setOption({
    tooltip: { trigger: 'axis' },
    legend: { data: ['下载', '上传'] },
    grid: { left: 60, right: 16, top: 32, bottom: 26 },
    xAxis: { type: 'time' },
    yAxis: { type: 'value', axisLabel: { formatter: (v: number) => fmtBytes(v) } },
    series: [
      { name: '下载', type: 'line', showSymbol: false, smooth: true, areaStyle: { opacity: 0.12 }, data: hist.map(([t, s]) => [t, s.recv]) },
      { name: '上传', type: 'line', showSymbol: false, smooth: true, areaStyle: { opacity: 0.12 }, data: hist.map(([t, s]) => [t, s.sent]) }
    ]
  })
}

function onResize() {
  cpuChart?.resize()
  netChart?.resize()
}

onMounted(() => {
  refresh()
  timer = window.setInterval(refresh, 3000)
  window.addEventListener('resize', onResize)
})

onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
  window.removeEventListener('resize', onResize)
  cpuChart?.dispose()
  netChart?.dispose()
})
</script>

<style scoped>
.metric { font-size: 26px; font-weight: 700; margin-bottom: 10px; }
.metric .sub { font-size: 13px; color: #999; font-weight: 400; }
.metric.hot { color: #e6423c; }
.sub { color: #888; font-size: 13px; line-height: 1.8; }
</style>
