<template>
  <div>
    <el-card class="page-card">
      <div class="toolbar">
        <el-button type="primary" @click="openCreate">新建任务</el-button>
        <el-button @click="load">刷新</el-button>
        <span class="hint">内置调度引擎运行，任务不写入系统 crontab</span>
      </div>
      <el-table :data="tasks" v-loading="loading">
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="name" label="任务名称" min-width="140" />
        <el-table-column label="类型" width="110">
          <template #default="{ row }"><el-tag size="small">{{ typeLabel(row.type) }}</el-tag></template>
        </el-table-column>
        <el-table-column prop="spec" label="执行周期" width="130" />
        <el-table-column label="下次执行" width="170">
          <template #default="{ row }">{{ row.enabled ? fmtTime(row.next_run) : '-' }}</template>
        </el-table-column>
        <el-table-column label="上次状态" width="100">
          <template #default="{ row }">
            <el-tag v-if="row.last_status" :type="statusType(row.last_status)" size="small">{{ row.last_status }}</el-tag>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="启用" width="80">
          <template #default="{ row }"><el-switch :model-value="row.enabled" @change="toggle(row)" /></template>
        </el-table-column>
        <el-table-column label="操作" width="270" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="success" @click="runNow(row)">执行</el-button>
            <el-button size="small" @click="openLogs(row)">日志</el-button>
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dlg" :title="form.id ? '编辑任务' : '新建任务'" width="660px">
      <el-form label-width="100px">
        <el-form-item label="任务名称"><el-input v-model="form.name" placeholder="如：每日备份数据库" /></el-form-item>
        <el-form-item label="任务类型">
          <el-select v-model="form.type" style="width: 100%">
            <el-option v-for="(v, k) in typeMap" :key="k" :label="v" :value="k" />
          </el-select>
        </el-form-item>
        <el-form-item label="执行周期">
          <el-input v-model="form.spec" placeholder="分 时 日 月 周，如 */5 * * * *；支持 @daily 等" />
          <div style="margin-top: 6px">
            <el-tag v-for="p in presets" :key="p.v" size="small" class="preset" @click="form.spec = p.v">{{ p.k }}</el-tag>
          </div>
        </el-form-item>
        <el-form-item v-if="form.type === 'shell'" label="脚本内容">
          <el-input v-model="form.command" type="textarea" :rows="8" placeholder="bash 命令，如：&#10;tar -czf /tmp/backup.tar.gz /data" />
        </el-form-item>
        <template v-if="form.type === 'backup_dir'">
          <el-form-item label="备份目录"><el-input v-model="form.source" placeholder="/data/www" /></el-form-item>
          <el-form-item label="保留份数"><el-input-number v-model="form.keep" :min="0" /><span class="hint">超出自动清理，0=永久保留</span></el-form-item>
        </template>
        <el-form-item v-if="form.type === 'visit_url'" label="访问 URL">
          <el-input v-model="form.url" placeholder="https://example.com/health（非 2xx/3xx 视为失败）" />
        </el-form-item>
        <el-form-item label="超时(秒)"><el-input-number v-model="form.timeout_sec" :min="0" :step="60" /><span class="hint">0 = 不限制</span></el-form-item>
        <el-form-item label="允许并发"><el-switch v-model="form.concurrent" /><span class="hint">关闭时上一次未结束则跳过本次</span></el-form-item>
        <el-form-item label="失败告警"><el-input-number v-model="form.notify_fail" :min="0" /><span class="hint">连续失败 N 次后告警，0=不告警</span></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dlg = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="logsDlg" size="62%" :title="`执行日志 - ${logsTask?.name || ''}`">
      <div class="toolbar">
        <el-button size="small" type="warning" @click="stopRun">终止运行中实例</el-button>
        <el-button size="small" @click="loadLogs">刷新</el-button>
      </div>
      <el-table :data="logItems" v-loading="logsLoading" size="small">
        <el-table-column label="状态" width="90">
          <template #default="{ row }"><el-tag :type="statusType(row.status)" size="small">{{ row.status }}</el-tag></template>
        </el-table-column>
        <el-table-column label="触发" width="70"><template #default="{ row }">{{ row.trigger }}</template></el-table-column>
        <el-table-column label="开始时间" width="165"><template #default="{ row }">{{ fmtTime(row.start_at) }}</template></el-table-column>
        <el-table-column label="耗时" width="90"><template #default="{ row }">{{ fmtCost(row.cost_ms) }}</template></el-table-column>
        <el-table-column prop="exit_code" label="退出码" width="75" />
        <el-table-column label="操作" width="90">
          <template #default="{ row }"><el-button size="small" @click="viewLog(row)">输出</el-button></template>
        </el-table-column>
      </el-table>
      <el-pagination style="margin-top: 12px; justify-content: flex-end" layout="prev, pager, next, total" :total="logTotal" :page-size="logSize" v-model:current-page="logPage" @current-change="loadLogs" />
    </el-drawer>

    <el-dialog v-model="outDlg" title="任务输出" width="760px">
      <pre class="pre-output">{{ outContent }}</pre>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'
import { fmtTime, fmtCost } from '../utils'

const typeMap: Record<string, string> = { shell: 'Shell 脚本', backup_dir: '目录备份', visit_url: '访问 URL' }
const typeLabel = (t: string) => typeMap[t] || t

const presets = [
  { k: '每分钟', v: '* * * * *' },
  { k: '每5分钟', v: '*/5 * * * *' },
  { k: '每小时', v: '0 * * * *' },
  { k: '每天 03:00', v: '0 3 * * *' },
  { k: '每周日 03:00', v: '0 3 * * 0' },
  { k: '每月1日 03:00', v: '0 3 1 * *' }
]

function statusType(s: string) {
  const map: Record<string, string> = { success: 'success', failed: 'danger', timeout: 'warning', stopped: 'warning', running: 'primary', skipped: 'info' }
  return (map as any)[s] || 'info'
}

const tasks = ref<any[]>([])
const loading = ref(false)
const dlg = ref(false)
const saving = ref(false)

const emptyForm = () => ({ id: 0, name: '', type: 'shell', spec: '*/5 * * * *', command: '', source: '', keep: 1, url: '', timeout_sec: 0, concurrent: false, notify_fail: 0 })
const form = reactive(emptyForm())

async function load() {
  loading.value = true
  try {
    tasks.value = (await api.get('/tasks')) as any
  } finally {
    loading.value = false
  }
}

function openCreate() {
  Object.assign(form, emptyForm())
  dlg.value = true
}

function openEdit(row: any) {
  Object.assign(form, emptyForm(), {
    id: row.id, name: row.name, type: row.type, spec: row.spec, command: row.command,
    timeout_sec: row.timeout_sec, concurrent: row.concurrent, notify_fail: row.notify_fail
  })
  try {
    const p = row.params ? JSON.parse(row.params) : {}
    form.source = p.source || ''
    form.keep = p.keep ?? 1
    form.url = p.url || ''
  } catch {
    /* ignore */
  }
  dlg.value = true
}

function payload() {
  const base: any = { name: form.name, type: form.type, spec: form.spec, timeout_sec: form.timeout_sec, concurrent: form.concurrent, notify_fail: form.notify_fail }
  if (form.type === 'shell') base.command = form.command
  else if (form.type === 'backup_dir') base.params = JSON.stringify({ source: form.source, keep: form.keep })
  else if (form.type === 'visit_url') base.params = JSON.stringify({ url: form.url })
  return base
}

async function save() {
  saving.value = true
  try {
    if (form.id) await api.put(`/tasks/${form.id}`, payload())
    else await api.post('/tasks', payload())
    dlg.value = false
    ElMessage.success('已保存')
    load()
  } finally {
    saving.value = false
  }
}

async function toggle(row: any) {
  await api.post(`/tasks/${row.id}/toggle`)
  load()
}

async function runNow(row: any) {
  await api.post(`/tasks/${row.id}/run`)
  ElMessage.success('已触发执行')
  setTimeout(load, 800)
}

async function del(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除任务「${row.name}」？执行历史将一并删除`, '删除任务', { type: 'warning' })
  } catch {
    return
  }
  await api.delete(`/tasks/${row.id}`)
  load()
}

const logsDlg = ref(false)
const logsTask = ref<any>(null)
const logItems = ref<any[]>([])
const logsLoading = ref(false)
const logPage = ref(1)
const logSize = 10
const logTotal = ref(0)
const outDlg = ref(false)
const outContent = ref('')

function openLogs(row: any) {
  logsTask.value = row
  logsDlg.value = true
  logPage.value = 1
  loadLogs()
}

async function loadLogs() {
  if (!logsTask.value) return
  logsLoading.value = true
  try {
    const d: any = await api.get(`/tasks/${logsTask.value.id}/logs`, { params: { page: logPage.value, size: logSize } })
    logItems.value = d.items
    logTotal.value = d.total
  } finally {
    logsLoading.value = false
  }
}

async function viewLog(row: any) {
  const d: any = await api.get(`/task-logs/${row.id}`)
  outContent.value = d.output || '（无输出）'
  outDlg.value = true
}

async function stopRun() {
  if (!logsTask.value) return
  await api.delete(`/tasks/${logsTask.value.id}/running`)
  ElMessage.success('已发送终止信号')
  setTimeout(loadLogs, 800)
}

onMounted(load)
</script>

<style scoped>
.preset { cursor: pointer; margin-right: 6px; }
.hint { color: #999; font-size: 12px; margin-left: 8px; }
</style>
