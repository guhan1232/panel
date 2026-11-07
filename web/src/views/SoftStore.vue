<template>
  <div class="wm-page">
    <WmSectionCard title="软件商店" description="原生二进制应用一键部署：下载源可选官方直连、GitHub 加速或阿里云/腾讯云/清华/中科大镜像站" flush>
      <WmToolbar>
        <WmStatusPill :label="`${templates.length} 个软件`" tone="info" :dot="false" />
        <el-button size="small" @click="load">刷新</el-button>
        <template #note>服务类应用由 systemd 托管（webmblx-app-*）；工具类（Node.js/Go 等）解压后软链到 /usr/local/bin。服务日志走 journalctl。</template>
      </WmToolbar>

      <div class="wm-section-block">
        <div class="wm-section-block__head">
          <span class="wm-eyebrow">可安装软件</span>
          <span class="wm-text-muted">内置 {{ builtinCount }} · 镜像源 {{ remoteCount }} 个</span>
        </div>
        <div class="wm-tpl-grid" v-if="templates.length">
          <article v-for="t in templates" :key="t.key" class="wm-tpl-card">
            <div class="wm-tpl-card__top">
              <h3 class="wm-tpl-card__name">{{ t.name }}</h3>
              <div class="wm-tpl-card__pills">
                <WmStatusPill :label="t.tool ? '工具' : '服务'" :tone="t.tool ? 'info' : 'warning'" :dot="false" outline />
                <WmStatusPill :label="t.source === 'remote' ? '镜像源' : '内置'" :tone="t.source === 'remote' ? 'danger' : 'success'" :dot="false" outline />
              </div>
            </div>
            <p class="wm-tpl-card__desc">{{ t.desc }}<template v-if="t.version"> · v{{ t.version }}</template></p>
            <el-button size="small" type="primary" class="wm-tpl-card__btn" @click="openInstall(t)">安装</el-button>
          </article>
        </div>
        <WmEmptyState v-else compact title="加载中" description="正在读取软件清单。" />
      </div>

      <div class="wm-section-block wm-section-block--table">
        <div class="wm-section-block__head">
          <span class="wm-eyebrow">已安装软件</span>
          <span class="wm-text-muted">{{ apps.length }} 个实例</span>
        </div>
        <WmTable :data="apps" v-loading="loading" empty-title="还没有安装软件" empty-description="从上方选择软件，选择下载源即可安装。">
          <el-table-column label="实例名" min-width="140"><template #default="{ row }"><b>{{ row.app.name }}</b></template></el-table-column>
          <el-table-column label="软件" width="130"><template #default="{ row }"><span class="wm-mono">{{ row.app.app_key }}</span></template></el-table-column>
          <el-table-column label="形态" width="90">
            <template #default="{ row }"><WmStatusPill :label="row.app.tool ? '工具' : '服务'" :tone="row.app.tool ? 'info' : 'warning'" :dot="false" outline /></template>
          </el-table-column>
          <el-table-column label="状态" min-width="200"><template #default="{ row }"><span class="mono">{{ row.status }}</span></template></el-table-column>
          <el-table-column label="安装时间" width="170"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.app.created_at) }}</span></template></el-table-column>
          <el-table-column label="操作" width="310" fixed="right">
            <template #default="{ row }">
              <template v-if="!row.app.tool">
                <el-button size="small" type="success" @click="act(row, 'start')">启动</el-button>
                <el-button size="small" type="warning" @click="act(row, 'stop')">停止</el-button>
                <el-button size="small" @click="act(row, 'restart')">重启</el-button>
              </template>
              <el-button size="small" @click="viewLogs(row)">日志</el-button>
              <el-button size="small" type="danger" @click="uninstall(row)">卸载</el-button>
            </template>
          </el-table-column>
        </WmTable>
      </div>
    </WmSectionCard>

    <el-dialog v-model="dlg" :title="`安装 ${cur?.name || ''}`" width="560px">
      <el-form label-width="110px">
        <el-form-item label="实例名称"><el-input v-model="instName" placeholder="如 prod-node" /></el-form-item>
        <el-form-item label="下载源">
          <el-select v-model="instSource" style="width: 100%">
            <el-option v-for="s in sources" :key="s.id" :label="s.name"
              :value="s.id" :disabled="cur && cur.soft_sources && !cur.soft_sources.includes(s.id)" />
          </el-select>
          <div class="hint">镜像站仅收录部分软件（未收录的源会置灰）；GitHub 类发布走加速通道</div>
        </el-form-item>
        <el-form-item v-for="p in cur?.params || []" :key="p.key" :label="p.label">
          <el-input v-model="instParams[p.key]" :placeholder="p.def" :type="p.key.includes('pass') || p.key === 'password' ? 'password' : 'text'" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dlg = false">取消</el-button>
        <el-button type="primary" :loading="deploying" @click="deploy">安装（下载发行包可能较慢）</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="logDlg" :title="`日志 - ${logName}`" width="820px">
      <WmCodeBlock :content="logContent" :label="`${logName} / logs`" />
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'
import { fmtTime } from '../utils'

const templates = ref<any[]>([])
const apps = ref<any[]>([])
const sources = ref<any[]>([])
const loading = ref(false)
const dlg = ref(false)
const deploying = ref(false)
const cur = ref<any>(null)
const instName = ref('')
const instSource = ref('auto')
const instParams = reactive<Record<string, string>>({})
const logDlg = ref(false)
const logName = ref('')
const logContent = ref('')

const builtinCount = computed(() => templates.value.filter((t) => t.source !== 'remote').length)
const remoteCount = computed(() => templates.value.filter((t) => t.source === 'remote').length)

async function load() {
  loading.value = true
  try {
    templates.value = ((await api.get('/apps/templates')) as any).filter((t: any) => t.kind === 'native')
    apps.value = ((await api.get('/apps')) as any).filter((a: any) => a.app.kind === 'native')
    sources.value = (await api.get('/apps/soft-sources')) as any
  } finally {
    loading.value = false
  }
}

function openInstall(t: any) {
  cur.value = t
  instName.value = t.key
  instSource.value = 'auto'
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
    await api.post('/apps', { app_key: cur.value.key, name: instName.value, mirror: instSource.value, params: { ...instParams } }, { timeout: 900000 })
    ElMessage.success('安装完成')
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
    await ElMessageBox.confirm(`确认卸载「${row.app.name}」？`, '卸载', { type: 'warning' })
  } catch {
    return
  }
  try {
    await ElMessageBox.confirm('是否同时删除安装目录（不可恢复）？', '删除文件', {
      type: 'error', confirmButtonText: '删除文件', cancelButtonText: '保留文件'
    })
    removeData = true
  } catch {
    removeData = false
  }
  await api.post(`/apps/${row.app.id}/uninstall`, { remove_data: removeData })
  ElMessage.success('已卸载')
  load()
}

onMounted(load)
</script>

<style scoped>
.wm-tpl-card__pills {
  display: flex;
  gap: 4px;
  align-items: center;
}
.mono {
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
