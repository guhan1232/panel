<template>
  <div class="wm-page">
    <WmSectionCard title="进程与服务" description="实时列表与 systemd 单元控制">
      <a-segmented v-model:value="tab" class="wm-mb-4" :options="[{ value: 'procs', label: '进程' }, { value: 'services', label: 'systemd 服务' }]" />
        <template v-if="tab === 'procs'">
          <WmToolbar>
            <a-segmented v-model:value="sortBy" :options="[{ value: 'cpu', label: '按 CPU' }, { value: 'mem', label: '按内存' }]" size="small" @change="loadProcs" />
            <span class="proc-auto">
              <a-switch v-model:checked="auto" size="small" />
              <span class="wm-text-muted">自动刷新</span>
            </span>
            <template #end>
              <a-button size="small" @click="loadProcs">刷新</a-button>
            </template>
            <template #note>共 {{ procs.length }} 个进程 · 开启自动刷新后每 5 秒采样</template>
          </WmToolbar>
          <WmTable :data="procs" v-loading="pLoading" size="small" max-height="620" empty-title="暂无进程数据">
            <WmColumn prop="pid" label="PID" width="100"><template #default="{ row }"><span class="wm-mono">{{ row.pid }}</span></template></WmColumn>
            <WmColumn prop="name" label="进程名" min-width="220" />
            <WmColumn label="CPU%" width="120"><template #default="{ row }"><span class="num-r" :class="{ hot: row.cpu > 60 }">{{ row.cpu.toFixed(1) }}</span></template></WmColumn>
            <WmColumn label="内存%" width="120"><template #default="{ row }"><span class="num-r">{{ row.mem_pct.toFixed(1) }}</span></template></WmColumn>
            <WmColumn label="操作" width="110">
              <template #default="{ row }">
                <a-button type="link" size="small" @click="kill(row, false)">结束</a-button>
                <a-button type="link" size="small" danger @click="kill(row, true)">强杀</a-button>
              </template>
            </WmColumn>
          </WmTable>
        </template>

        <template v-else>
          <WmToolbar>
            <template #end>
              <a-button size="small" @click="loadSvcs">刷新</a-button>
            </template>
            <template #note>共 {{ svcs.length }} 个服务单元</template>
          </WmToolbar>
          <WmTable :data="svcs" v-loading="sLoading" size="small" max-height="620" empty-title="暂无服务数据">
            <WmColumn prop="unit" label="服务" min-width="230"><template #default="{ row }"><span class="wm-mono">{{ row.unit }}</span></template></WmColumn>
            <WmColumn prop="active" label="状态" width="110">
              <template #default="{ row }"><WmStatusPill :label="row.active" :tone="row.active === 'active' ? 'success' : 'muted'" /></template>
            </WmColumn>
            <WmColumn prop="sub" label="子状态" width="110" />
            <WmColumn prop="enabled" label="自启" width="110"><template #default="{ row }"><span class="wm-mono">{{ row.enabled }}</span></template></WmColumn>
            <WmColumn prop="description" label="描述" min-width="230" />
            <WmColumn label="操作" width="190">
              <template #default="{ row }">
                <a-button type="link" size="small" :disabled="row.active === 'active'" @click="svcAction(row, 'start')">启动</a-button>
                <a-button type="link" size="small" :disabled="row.active !== 'active'" @click="svcAction(row, 'stop')">停止</a-button>
                <a-dropdown>
                  <a-button type="link" size="small">更多<span class="wm-icon wm-dd-icon"><ArrowDown /></span></a-button>
                  <template #overlay>
                    <a-menu>
                      <a-menu-item key="restart" @click="svcAction(row, 'restart')">重启</a-menu-item>
                      <a-menu-item key="enable" @click="svcAction(row, 'enable')">设为自启</a-menu-item>
                      <a-menu-item key="disable" @click="svcAction(row, 'disable')">禁用自启</a-menu-item>
                    </a-menu>
                  </template>
                </a-dropdown>
              </template>
            </WmColumn>
          </WmTable>
        </template>
      
    </WmSectionCard>
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from '../antd-compat'
import { ArrowDown } from '../antd-compat'
import api from '../api'

const tab = ref('procs')
const procs = ref<any[]>([])
const pLoading = ref(false)
const sortBy = ref('cpu')
const auto = ref(true)
const svcs = ref<any[]>([])
const sLoading = ref(false)
let timer: number | null = null

async function loadProcs() {
  pLoading.value = true
  try {
    procs.value = (await api.get('/proc/list', { params: { sort: sortBy.value } })) as any
  } finally {
    pLoading.value = false
  }
}

async function loadSvcs() {
  sLoading.value = true
  try {
    svcs.value = (await api.get('/proc/services')) as any
  } finally {
    sLoading.value = false
  }
}

async function kill(row: any, force: boolean) {
  try {
    await ElMessageBox.confirm(`确认${force ? '强制杀死' : '结束'}进程 ${row.pid} (${row.name})？`, '确认', { type: 'warning' })
  } catch {
    return
  }
  await api.post('/proc/kill', { pid: row.pid, force })
  ElMessage.success('已发送信号')
  setTimeout(loadProcs, 500)
}

async function svcAction(row: any, action: string) {
  try {
    await ElMessageBox.confirm(`确认对服务 ${row.unit} 执行 ${action}？`, '确认', { type: 'warning' })
  } catch {
    return
  }
  await api.post('/proc/service', { name: row.unit, action })
  ElMessage.success('已执行')
  setTimeout(loadSvcs, 800)
}

watch(tab, (t) => {
  if (t === 'services' && !svcs.value.length) loadSvcs()
})

onMounted(() => {
  loadProcs()
  timer = window.setInterval(() => {
    if (auto.value && tab.value === 'procs') loadProcs()
  }, 5000)
})

onBeforeUnmount(() => {
  if (timer) window.clearInterval(timer)
})
</script>

<style scoped>
.proc-auto { display: inline-flex; align-items: center; gap: 6px; font-size: 12px; }
.num-r { display: block; text-align: right; font-family: var(--wm-font-mono); font-variant-numeric: tabular-nums; }
.wm-dd-icon { margin-left: 4px; font-size: 10px; }
</style>
