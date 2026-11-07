<template>
  <div>
    <el-card class="page-card">
      <div class="toolbar">
        <el-tag :type="docker.installed ? (docker.compose ? 'success' : 'warning') : 'danger'" size="small">
          Docker {{ docker.installed ? (docker.compose ? '就绪' : '缺少 compose 插件') : '未安装（仅影响容器类模板）' }}
        </el-tag>
        <el-button v-if="!docker.installed" size="small" type="warning" :loading="installing" @click="installDocker">一键安装 Docker</el-button>
        <el-button size="small" @click="load">刷新</el-button>
        <span class="hint">原生应用经 systemd 托管（webmblx-app-*），日志走 journalctl；容器应用走 Docker Compose</span>
      </div>
      <h4 style="margin: 12px 0 8px">可用模板</h4>
      <el-row :gutter="12">
        <el-col :span="6" v-for="t in templates" :key="t.key">
          <el-card shadow="hover" style="margin-bottom: 12px">
            <b>{{ t.name }}</b>
            <el-tag size="small" :type="t.kind === 'native' ? 'warning' : 'primary'" style="margin-left: 6px">
              {{ t.kind === 'native' ? '原生' : '容器' }}
            </el-tag>
            <div class="hint" style="margin: 6px 0">{{ t.desc }}</div>
            <el-button size="small" type="primary"
              :disabled="t.kind === 'compose' && (!docker.installed || !docker.compose)"
              @click="openInstall(t)">部署</el-button>
          </el-card>
        </el-col>
      </el-row>
      <h4 style="margin: 4px 0 8px">已部署应用</h4>
      <el-table :data="apps" v-loading="loading">
        <el-table-column label="实例名" min-width="130"><template #default="{ row }"><b>{{ row.app.name }}</b></template></el-table-column>
        <el-table-column label="应用" width="110"><template #default="{ row }"><el-tag size="small">{{ row.app.app_key }}</el-tag></template></el-table-column>
        <el-table-column label="形态" width="80">
          <template #default="{ row }">
            <el-tag size="small" :type="row.app.kind === 'native' ? 'warning' : 'primary'">{{ row.app.kind === 'native' ? '原生' : '容器' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" min-width="240"><template #default="{ row }"><span class="mono">{{ row.status }}</span></template></el-table-column>
        <el-table-column label="部署时间" width="165"><template #default="{ row }">{{ fmtTime(row.app.created_at) }}</template></el-table-column>
        <el-table-column label="操作" width="310" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="success" @click="act(row, 'start')">启动</el-button>
            <el-button size="small" type="warning" @click="act(row, 'stop')">停止</el-button>
            <el-button size="small" @click="act(row, 'restart')">重启</el-button>
            <el-button size="small" @click="viewLogs(row)">日志</el-button>
            <el-button size="small" type="danger" @click="uninstall(row)">卸载</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dlg" :title="`部署 ${cur?.name || ''}`" width="520px">
      <el-form label-width="130px">
        <el-form-item label="实例名称"><el-input v-model="instName" placeholder="如 my-redis" /></el-form-item>
        <el-form-item v-for="p in cur?.params || []" :key="p.key" :label="p.label">
          <el-input v-model="instParams[p.key]" :placeholder="p.def" :type="p.key.includes('pass') || p.key === 'password' ? 'password' : 'text'" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dlg = false">取消</el-button>
        <el-button type="primary" :loading="deploying" @click="deploy">部署（拉取镜像可能较慢）</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="logDlg" :title="`日志 - ${logName}`" width="820px">
      <pre class="pre-output">{{ logContent || '（空）' }}</pre>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'
import { fmtTime } from '../utils'

const docker = ref<any>({})
const templates = ref<any[]>([])
const apps = ref<any[]>([])
const loading = ref(false)
const installing = ref(false)
const dlg = ref(false)
const deploying = ref(false)
const cur = ref<any>(null)
const instName = ref('')
const instParams = reactive<Record<string, string>>({})
const logDlg = ref(false)
const logName = ref('')
const logContent = ref('')

async function load() {
  loading.value = true
  try {
    docker.value = await api.get('/apps/docker/status')
    templates.value = (await api.get('/apps/templates')) as any
    apps.value = (await api.get('/apps')) as any
  } finally {
    loading.value = false
  }
}

function openInstall(t: any) {
  cur.value = t
  instName.value = t.key
  Object.keys(instParams).forEach((k) => delete instParams[k])
  for (const p of t.params || []) instParams[p.key] = p.def || ''
  dlg.value = true
}

async function deploy() {
  if (!instName.value) {
    ElMessage.warning('请填写实例名称')
    return
  }
  deploying.value = true
  try {
    await api.post('/apps', { app_key: cur.value.key, name: instName.value, params: { ...instParams } }, { timeout: 900000 })
    ElMessage.success('部署完成')
    dlg.value = false
    load()
  } finally {
    deploying.value = false
  }
}

async function act(row: any, action: string) {
  await api.post(`/apps/${row.app.id}/${action}`, {}, { timeout: 900000 })
  ElMessage.success('已执行 ' + action)
  load()
}

async function viewLogs(row: any) {
  const d: any = await api.get(`/apps/${row.app.id}/logs`)
  logName.value = row.app.name
  logContent.value = d.content
  logDlg.value = true
}

async function uninstall(row: any) {
  let removeData = false
  try {
    await ElMessageBox.confirm(`确认卸载应用「${row.app.name}」？`, '卸载', { type: 'warning' })
  } catch {
    return
  }
  try {
    await ElMessageBox.confirm('是否同时删除数据目录（数据库文件等，不可恢复）？', '删除数据', {
      type: 'error', confirmButtonText: '删除数据', cancelButtonText: '保留数据'
    })
    removeData = true
  } catch {
    removeData = false
  }
  await api.post(`/apps/${row.app.id}/uninstall`, { remove_data: removeData })
  ElMessage.success('已卸载')
  load()
}

async function installDocker() {
  try {
    await ElMessageBox.confirm('将通过官方脚本安装 Docker（需要外网，耗时数分钟），继续？', '安装 Docker', { type: 'warning' })
  } catch {
    return
  }
  installing.value = true
  try {
    await api.post('/apps/docker/install', {}, { timeout: 1800000 })
    ElMessage.success('Docker 安装完成')
    load()
  } finally {
    installing.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.mono { font-family: Consolas, Monaco, monospace; font-size: 12px; white-space: pre-wrap; word-break: break-all; }
.hint { color: #999; font-size: 12px; }
</style>
