<template>
  <div class="wm-page">
    <WmSectionCard title="任务列表" description="任务定义持久化于面板数据库，重启后自动恢复调度" flush>
      <template #actions>
        <span class="wm-text-muted wm-row-stats">{{ tasks.length }} 条 · {{ enabledCount }} 启用</span>
      </template>
      <WmToolbar>
        <template #end>
          <a-button type="primary" size="small" @click="openCreate">新建任务</a-button>
          <a-button size="small" @click="load">刷新</a-button>
        </template>
        <template #note>内置调度引擎运行，任务不写入系统 crontab。</template>
      </WmToolbar>

      <WmTable :data="tasks" v-loading="loading" empty-title="还没有计划任务" empty-description="新建一条任务，按 cron 表达式或预设周期自动执行。">
        <template #empty>
          <WmEmptyState compact title="还没有计划任务" description="新建一条任务，按 cron 表达式或预设周期自动执行；支持 Shell、备份、日志切割等类型。">
            <template #action>
              <a-button type="primary" @click="openCreate">新建任务</a-button>
            </template>
          </WmEmptyState>
        </template>
        <WmColumn prop="id" label="ID" width="60"><template #default="{ row }"><span class="wm-mono">{{ row.id }}</span></template></WmColumn>
        <WmColumn prop="name" label="任务名称" min-width="150"><template #default="{ row }"><b>{{ row.name }}</b></template></WmColumn>
        <WmColumn label="类型" width="120">
          <template #default="{ row }"><WmStatusPill :label="typeLabel(row.type)" tone="neutral" :dot="false" outline /></template>
        </WmColumn>
        <WmColumn prop="spec" label="执行周期" width="140"><template #default="{ row }"><span class="wm-mono">{{ row.spec }}</span></template></WmColumn>
        <WmColumn label="下次执行" width="170">
          <template #default="{ row }"><span class="wm-mono">{{ row.enabled ? fmtTime(row.next_run) : '-' }}</span></template>
        </WmColumn>
        <WmColumn label="上次状态" width="110">
          <template #default="{ row }">
            <WmStatusPill v-if="row.last_status" :label="row.last_status" :pulse="row.last_status === 'running'" />
            <span v-else class="wm-text-muted">—</span>
          </template>
        </WmColumn>
        <WmColumn label="启用" width="80">
          <template #default="{ row }"><a-switch :checked="row.enabled" @change="toggle(row)" /></template>
        </WmColumn>
        <WmColumn label="操作" width="200" fixed="right">
          <template #default="{ row }">
            <a-button type="link" size="small" @click="runNow(row)">执行</a-button>
            <a-button type="link" size="small" @click="openLogs(row)">日志</a-button>
            <a-button type="link" size="small" @click="openEdit(row)">编辑</a-button>
            <a-button type="link" size="small" danger @click="del(row)">删除</a-button>
          </template>
        </WmColumn>
      </WmTable>
    </WmSectionCard>

    <a-drawer v-model:open="dlg" :title="form.id ? '编辑任务' : '新建任务'" :width="680">
      <WmNotice tone="neutral" class="wm-mb-4">周期支持 5 段 cron、带秒 6 段与 @daily 等描述符；同一任务默认防重叠执行。</WmNotice>
      <a-form layout="vertical">
        <div class="wm-form-sec">
          <div class="wm-form-sec__title">基本信息</div>
          <a-form-item label="任务名称"><a-input v-model:value="form.name" placeholder="如：每日备份数据库" /></a-form-item>
          <a-form-item label="任务类型">
            <a-select v-model:value="form.type" style="width: 100%">
              <a-select-option v-for="(v, k) in typeMap" :key="k" :value="k">{{ v }}</a-select-option>
            </a-select>
          </a-form-item>
          <a-form-item label="执行周期" style="margin-bottom: 0">
            <a-input v-model:value="form.spec" placeholder="分 时 日 月 周，如 */5 * * * *；支持 @daily 等" />
            <div class="wm-tag-row">
              <a-tag v-for="p in presets" :key="p.v" class="preset" @click="form.spec = p.v">{{ p.k }}</a-tag>
            </div>
          </a-form-item>
        </div>
        <div class="wm-form-sec">
          <div class="wm-form-sec__title">任务配置</div>
          <a-form-item v-if="form.type === 'shell'" label="执行位置">
            <a-select v-model:value="form.node_id" style="width: 100%">
              <a-select-option :value="0">本机（面板所在服务器）</a-select-option>
              <a-select-option v-for="nd in nodes" :key="nd.id" :value="nd.id">{{ nd.name }}（远程）</a-select-option>
            </a-select>
            <div class="hint">远程执行经 Agent 通道下发；离线节点任务会执行失败。</div>
          </a-form-item>
          <a-form-item v-if="form.type === 'shell'" label="脚本内容">
            <a-textarea v-model:value="form.command"  :rows="8" placeholder="bash 命令，如：&#10;tar -czf /tmp/backup.tar.gz /data" />
          </a-form-item>
          <template v-if="form.type === 'backup_dir'">
            <a-form-item label="备份目录"><a-input v-model:value="form.source" placeholder="/data/www" /></a-form-item>
            <a-form-item label="保留份数"><a-input-number v-model:value="form.keep" :min="0" /><span class="hint">超出自动清理，0=永久保留</span></a-form-item>
          </template>
          <a-form-item v-if="form.type === 'visit_url'" label="访问 URL">
            <a-input v-model:value="form.url" placeholder="https://example.com/health（非 2xx/3xx 视为失败）" />
          </a-form-item>
          <template v-if="form.type === 'backup_db'">
            <a-form-item label="数据库类型">
              <a-radio-group v-model:value="form.db_engine" option-type="button" button-style="solid">
                <a-radio-button value="mysql">MySQL</a-radio-button>
                <a-radio-button value="pg">PostgreSQL</a-radio-button>
              </a-radio-group>
            </a-form-item>
            <a-form-item label="数据库名"><a-input v-model:value="form.db_name" placeholder="mydb" /></a-form-item>
            <a-form-item label="保留份数"><a-input-number v-model:value="form.keep" :min="0" /><span class="hint">连接凭据使用「数据库」页面保存的配置</span></a-form-item>
          </template>
          <template v-if="form.type === 'log_rotate'">
            <a-form-item label="日志匹配"><a-input v-model:value="form.pattern" placeholder="/var/log/nginx/*.log（不含空格）" /></a-form-item>
            <a-form-item label="切割阈值(MB)"><a-input-number v-model:value="form.size_mb" :min="1" /></a-form-item>
            <a-form-item label="保留天数"><a-input-number v-model:value="form.keep_days" :min="1" /></a-form-item>
          </template>
          <WmNotice v-if="form.type === 'sys_update'" tone="warning" title="请在低峰期执行">将执行 apt-get upgrade / dnf upgrade 全量系统更新。</WmNotice>
        </div>
        <div class="wm-form-sec">
          <div class="wm-form-sec__title">执行策略</div>
          <a-form-item label="超时(秒)"><a-input-number v-model:value="form.timeout_sec" :min="0" :step="60" /><span class="hint">0 = 不限制</span></a-form-item>
          <a-form-item label="允许并发">
            <div><a-switch v-model:checked="form.concurrent" /></div>
            <div class="hint">关闭时上一次未结束则跳过本次</div>
          </a-form-item>
          <a-form-item label="失败告警" style="margin-bottom: 0"><a-input-number v-model:value="form.notify_fail" :min="0" /><span class="hint">连续失败 N 次后告警，0=不告警</span></a-form-item>
        </div>
      </a-form>
      <template #footer>
        <a-button @click="dlg = false">取消</a-button>
        <a-button type="primary" :loading="saving" @click="save">保存</a-button>
      </template>
    </a-drawer>

    <a-drawer v-model:open="logsDlg" width="860px" :title="`执行日志 - ${logsTask?.name || ''}`">
      <WmNotice tone="neutral" class="wm-mb-4">每次执行记录状态、耗时、退出码与输出；历史保留 90 天后自动清理。</WmNotice>
      <WmToolbar>
        <a-button size="small" danger @click="stopRun">终止运行中实例</a-button>
        <a-button size="small" @click="loadLogs">刷新</a-button>
      </WmToolbar>
      <WmTable :data="logItems" v-loading="logsLoading" size="small" empty-title="暂无执行记录">
        <WmColumn label="状态" width="100">
          <template #default="{ row }"><WmStatusPill :label="row.status" /></template>
        </WmColumn>
        <WmColumn label="触发" width="80"><template #default="{ row }">{{ row.trigger }}</template></WmColumn>
        <WmColumn label="开始时间" width="170"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.start_at) }}</span></template></WmColumn>
        <WmColumn label="耗时" width="95"><template #default="{ row }"><span class="wm-mono">{{ fmtCost(row.cost_ms) }}</span></template></WmColumn>
        <WmColumn prop="exit_code" label="退出码" width="80"><template #default="{ row }"><span class="wm-mono">{{ row.exit_code }}</span></template></WmColumn>
        <WmColumn label="操作" width="90">
          <template #default="{ row }"><a-button size="small" @click="viewLog(row)">输出</a-button></template>
        </WmColumn>
      </WmTable>
      <a-pagination :hide-on-single-page="true" :total="logTotal" :page-size="logSize" v-model:current="logPage" @change="loadLogs" />
    </a-drawer>

    <a-modal v-model:open="outDlg" title="任务输出" width="680px">
      <WmCodeBlock :content="outContent" label="task output" />
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, Timer } from '../antd-compat'
import api from '../api'
import { fmtTime, fmtCost } from '../utils'

const typeMap: Record<string, string> = {
  shell: 'Shell 脚本', backup_dir: '目录备份', visit_url: '访问 URL',
  backup_db: '数据库备份', log_rotate: '日志切割', sys_update: '系统更新'
}
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
const enabledCount = computed(() => tasks.value.filter((t: any) => t.enabled).length)

const emptyForm = () => ({ id: 0, name: '', type: 'shell', spec: '*/5 * * * *', command: '', source: '', keep: 1, url: '', db_engine: 'mysql', db_name: '', pattern: '', size_mb: 50, keep_days: 30, timeout_sec: 0, concurrent: false, notify_fail: 0, node_id: 0 })
const form = reactive(emptyForm())
const nodes = ref<any[]>([])

async function loadNodes() {
  try {
    const list: any[] = (await api.get('/nodes')) as any
    nodes.value = list.map((x: any) => ({ id: x.node.id, name: x.node.name, online: x.online }))
  } catch {
    nodes.value = []
  }
}

onMounted(loadNodes)

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
    form.db_engine = p.engine || 'mysql'
    form.db_name = p.database || ''
    form.pattern = p.pattern || ''
    form.size_mb = p.size_mb || 50
    form.keep_days = p.keep_days || 30
  } catch {
    /* ignore */
  }
  dlg.value = true
}

function payload() {
  const base: any = { name: form.name, type: form.type, spec: form.spec, timeout_sec: form.timeout_sec, concurrent: form.concurrent, notify_fail: form.notify_fail, node_id: form.node_id }
  if (form.type === 'shell') base.command = form.command
  else if (form.type === 'backup_dir') base.params = JSON.stringify({ source: form.source, keep: form.keep })
  else if (form.type === 'visit_url') base.params = JSON.stringify({ url: form.url })
  else if (form.type === 'backup_db') base.params = JSON.stringify({ engine: form.db_engine, database: form.db_name, keep: form.keep })
  else if (form.type === 'log_rotate') base.params = JSON.stringify({ pattern: form.pattern, size_mb: form.size_mb, keep_days: form.keep_days })
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
