<template>
  <el-card>
    <el-tabs v-model="tab">
      <el-tab-pane label="进程" name="procs">
        <div class="toolbar">
          <el-radio-group v-model="sortBy" size="small" @change="loadProcs">
            <el-radio-button value="cpu">按 CPU 排序</el-radio-button>
            <el-radio-button value="mem">按内存排序</el-radio-button>
          </el-radio-group>
          <el-switch v-model="auto" active-text="自动刷新(5s)" />
          <el-button size="small" @click="loadProcs">刷新</el-button>
        </div>
        <el-table :data="procs" v-loading="pLoading" size="small" max-height="620">
          <el-table-column prop="pid" label="PID" width="90" />
          <el-table-column prop="name" label="进程名" min-width="200" />
          <el-table-column label="CPU%" width="110"><template #default="{ row }">{{ row.cpu.toFixed(1) }}</template></el-table-column>
          <el-table-column label="内存%" width="110"><template #default="{ row }">{{ row.mem_pct.toFixed(1) }}</template></el-table-column>
          <el-table-column label="操作" width="170">
            <template #default="{ row }">
              <el-button size="small" type="warning" @click="kill(row, false)">结束</el-button>
              <el-button size="small" type="danger" @click="kill(row, true)">强杀</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="systemd 服务" name="services">
        <div class="toolbar">
          <el-button size="small" @click="loadSvcs">刷新</el-button>
        </div>
        <el-table :data="svcs" v-loading="sLoading" size="small" max-height="620">
          <el-table-column prop="unit" label="服务" min-width="220" />
          <el-table-column prop="active" label="状态" width="100">
            <template #default="{ row }">
              <el-tag size="small" :type="row.active === 'active' ? 'success' : 'info'">{{ row.active }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="sub" label="子状态" width="100" />
          <el-table-column prop="enabled" label="自启" width="100" />
          <el-table-column prop="description" label="描述" min-width="220" />
          <el-table-column label="操作" width="300">
            <template #default="{ row }">
              <el-button size="small" :disabled="row.active === 'active'" @click="svcAction(row, 'start')">启动</el-button>
              <el-button size="small" :disabled="row.active !== 'active'" @click="svcAction(row, 'stop')">停止</el-button>
              <el-button size="small" @click="svcAction(row, 'restart')">重启</el-button>
              <el-button size="small" @click="svcAction(row, 'enable')">自启</el-button>
              <el-button size="small" @click="svcAction(row, 'disable')">禁自启</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>
  </el-card>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
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
