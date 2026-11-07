<template>
  <div class="wm-page">
    <WmSectionCard title="环境与应用" description="原生应用由 systemd 托管，容器应用由 Docker Compose 编排" flush>
      <WmToolbar>
        <WmStatusPill
          :label="docker.installed ? (docker.compose ? 'Docker 就绪' : '缺少 compose 插件') : 'Docker 未安装'"
          :tone="docker.installed ? (docker.compose ? 'success' : 'warning') : 'danger'"
        />
        <el-button v-if="!docker.installed" size="small" type="warning" :loading="installing" @click="installDocker">一键安装 Docker</el-button>
        <el-button size="small" @click="openMirrors">镜像源配置</el-button>
        <el-button v-if="auth.role === 'admin'" size="small" @click="openSource">应用源配置</el-button>
        <el-button size="small" @click="load">刷新</el-button>
        <template #note>原生应用日志走 journalctl；容器模板需要 Docker 与 compose 插件。安装时可选择镜像源加速。</template>
      </WmToolbar>

      <div class="wm-section-block">
        <div class="wm-section-block__head">
          <span class="wm-eyebrow">可用模板</span>
          <span class="wm-text-muted">内置 {{ builtinCount }} · 镜像源 {{ remoteCount }} · 共 {{ templates.length }} 个</span>
        </div>
        <div class="wm-tpl-grid" v-if="templates.length">
          <article v-for="t in templates" :key="t.key" class="wm-tpl-card">
            <div class="wm-tpl-card__top">
              <h3 class="wm-tpl-card__name">{{ t.name }}</h3>
              <div class="wm-tpl-card__pills">
                <WmStatusPill :label="t.kind === 'native' ? '原生' : '容器'" :tone="t.kind === 'native' ? 'warning' : 'info'" :dot="false" />
                <WmStatusPill :label="t.source === 'remote' ? '镜像源' : '内置'" :tone="t.source === 'remote' ? 'danger' : 'success'" :dot="false" outline />
              </div>
            </div>
            <p class="wm-tpl-card__desc">{{ t.desc }}<template v-if="t.version"> · v{{ t.version }}</template></p>
            <el-button size="small" type="primary" class="wm-tpl-card__btn"
              :disabled="t.kind === 'compose' && (!docker.installed || !docker.compose)"
              @click="openInstall(t)">部署</el-button>
          </article>
        </div>
        <WmEmptyState v-else compact title="模板加载中" description="正在从面板读取应用模板清单。" />
      </div>

      <div class="wm-section-block wm-section-block--table">
        <div class="wm-section-block__head">
          <span class="wm-eyebrow">已部署应用</span>
          <span class="wm-text-muted">{{ apps.length }} 个实例</span>
        </div>
        <WmTable :data="apps" v-loading="loading" empty-title="还没有部署应用" empty-description="从上方模板中选择一个，填写实例名称即可部署。">
          <el-table-column label="实例名" min-width="140"><template #default="{ row }"><b>{{ row.app.name }}</b></template></el-table-column>
          <el-table-column label="应用" width="120"><template #default="{ row }"><span class="wm-mono">{{ row.app.app_key }}</span></template></el-table-column>
          <el-table-column label="形态" width="90">
            <template #default="{ row }"><WmStatusPill :label="row.app.kind === 'native' ? '原生' : '容器'" :tone="row.app.kind === 'native' ? 'warning' : 'info'" :dot="false" outline /></template>
          </el-table-column>
          <el-table-column label="状态" min-width="240"><template #default="{ row }"><span class="mono">{{ row.status }}</span></template></el-table-column>
          <el-table-column label="部署时间" width="170"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.app.created_at) }}</span></template></el-table-column>
          <el-table-column label="操作" width="310" fixed="right">
            <template #default="{ row }">
              <el-button size="small" type="success" @click="act(row, 'start')">启动</el-button>
              <el-button size="small" type="warning" @click="act(row, 'stop')">停止</el-button>
              <el-button size="small" @click="act(row, 'restart')">重启</el-button>
              <el-button size="small" @click="viewLogs(row)">日志</el-button>
              <el-button size="small" type="danger" @click="uninstall(row)">卸载</el-button>
            </template>
          </el-table-column>
        </WmTable>
      </div>
    </WmSectionCard>

    <el-dialog v-model="dlg" :title="`部署 ${cur?.name || ''}`" width="520px">
      <el-form label-width="130px">
        <el-form-item label="实例名称"><el-input v-model="instName" placeholder="如 my-redis" /></el-form-item>
        <el-form-item label="镜像源">
          <el-select v-model="instMirror" style="width: 100%">
            <el-option v-for="m in mirrors" :key="m.id" :label="m.name" :value="m.id" />
          </el-select>
          <div class="hint">容器模板经所选源代理拉取镜像；原生模板经 GitHub 加速下载发行包；「自动」按应用类型选第一个可用加速源</div>
        </el-form-item>
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
      <WmCodeBlock :content="logContent" :label="`${logName} / logs`" />
    </el-dialog>

    <el-dialog v-model="srcDlg" title="应用源配置（远程模板目录）" width="620px">
      <el-form label-width="90px">
        <el-form-item label="目录 URL">
          <el-input v-model="srcUrl" placeholder="https://.../catalog.json（清空并保存即清除应用源）" />
        </el-form-item>
      </el-form>
      <WmNotice tone="info" title="清单格式">
        JSON 数组，每项为声明式应用定义（compose 文本或原生二进制描述），格式参考仓库 <span class="wm-mono">docs/catalog.example.json</span>。
        最多 50 个应用、单文件 1MB；拉取后合并进模板列表（key 与内置重复时内置优先）。远程清单由管理员提供，请确保来源可信。
      </WmNotice>
      <template #footer>
        <el-button @click="srcDlg = false">取消</el-button>
        <el-button type="danger" plain @click="clearSource">清除应用源</el-button>
        <el-button type="primary" :loading="srcSaving" @click="saveSource">保存并拉取</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="mirrorDlg" title="Docker 镜像源配置" width="680px">
      <WmNotice v-if="mirrorInfo.docker_installed === false" tone="warning" title="尚未检测到 Docker" class="wm-mb-4">可先编辑配置文件，安装 Docker 后生效。</WmNotice>
      <el-form label-width="110px">
        <el-form-item label="加速镜像源">
          <el-input v-model="mirrorForm.registry_mirrors" type="textarea" :rows="5" placeholder="每行一个，如 https://docker.m.daocloud.io" />
          <div class="presets">
            <span class="hint">常用源（时效性强，按可用性自行取舍）：</span>
            <el-tag v-for="m in mirrorPresets" :key="m" size="small" class="preset-tag" @click="addPreset(m)">{{ m }}</el-tag>
          </div>
        </el-form-item>
        <el-form-item label="私有仓库白名单">
          <el-input v-model="mirrorForm.insecure_registries" type="textarea" :rows="3" placeholder="每行一个 HTTP 私有仓库，如 192.168.1.10:5000" />
        </el-form-item>
        <el-form-item v-if="mirrorInfo.live_mirrors && mirrorInfo.live_mirrors.length" label="当前生效">
          <div class="live-mirrors">
            <WmStatusPill v-for="m in mirrorInfo.live_mirrors" :key="m" :label="m" tone="success" :dot="false" mono />
          </div>
        </el-form-item>
        <el-form-item label="配置文件"><span class="hint wm-mono">{{ mirrorInfo.path }}</span><span class="hint">其余配置项保持不变，修改前自动备份</span></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="mirrorDlg = false">取消</el-button>
        <el-button type="primary" :loading="mirrorSaving" @click="saveMirrors">保存并重启 Docker</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'
import { fmtTime } from '../utils'
import { useAuth } from '../stores/auth'

const auth = useAuth()
const docker = ref<any>({})
const templates = ref<any[]>([])
const apps = ref<any[]>([])
const loading = ref(false)
const installing = ref(false)
const dlg = ref(false)
const deploying = ref(false)
const cur = ref<any>(null)
const instName = ref('')
const instMirror = ref('auto')
const mirrors = ref<any[]>([])
const instParams = reactive<Record<string, string>>({})
const logDlg = ref(false)
const logName = ref('')
const logContent = ref('')

const builtinCount = computed(() => templates.value.filter((t) => t.source !== 'remote').length)
const remoteCount = computed(() => templates.value.filter((t) => t.source === 'remote').length)

const srcDlg = ref(false)
const srcUrl = ref('')
const srcSaving = ref(false)

async function openSource() {
  const d: any = await api.get('/apps/source')
  srcUrl.value = d.url || ''
  srcDlg.value = true
}

async function saveSource() {
  srcSaving.value = true
  try {
    const d: any = await api.post('/apps/source', { url: srcUrl.value.trim() }, { timeout: 60000 })
    ElMessage.success(`应用源已保存，拉取到 ${d.count} 个应用`)
    srcDlg.value = false
    load()
  } finally {
    srcSaving.value = false
  }
}

async function clearSource() {
  await api.post('/apps/source', { url: '' })
  ElMessage.success('应用源已清除')
  srcUrl.value = ''
  srcDlg.value = false
  load()
}

const mirrorPresets = [
  'https://docker.m.daocloud.io',
  'https://docker.1ms.run',
  'https://hub.rat.dev',
  'https://docker.nju.edu.cn',
  'https://docker.mirrors.ustc.edu.cn',
  'https://hub-mirror.c.163.com'
]
const mirrorDlg = ref(false)
const mirrorSaving = ref(false)
const mirrorInfo = ref<any>({})
const mirrorForm = reactive({ registry_mirrors: '', insecure_registries: '' })

async function openMirrors() {
  mirrorInfo.value = await api.get('/apps/docker/mirrors')
  mirrorForm.registry_mirrors = (mirrorInfo.value.registry_mirrors || []).join('\n')
  mirrorForm.insecure_registries = (mirrorInfo.value.insecure_registries || []).join('\n')
  mirrorDlg.value = true
}

function addPreset(m: string) {
  const lines = mirrorForm.registry_mirrors.split('\n').map((s: string) => s.trim()).filter(Boolean)
  if (!lines.includes(m)) lines.push(m)
  mirrorForm.registry_mirrors = lines.join('\n')
}

async function saveMirrors() {
  try {
    await ElMessageBox.confirm(
      '保存需要重启 Docker 服务，运行中的容器会短暂中断（restart=always 的容器会自动恢复），继续？',
      '重启确认', { type: 'warning' }
    )
  } catch {
    return
  }
  mirrorSaving.value = true
  try {
    const d: any = await api.post('/apps/docker/mirrors', {
      registry_mirrors: mirrorForm.registry_mirrors.split('\n').map((s: string) => s.trim()).filter(Boolean),
      insecure_registries: mirrorForm.insecure_registries.split('\n').map((s: string) => s.trim()).filter(Boolean),
      restart: true
    }, { timeout: 180000 })
    ElMessage.success(d.restarted ? '已保存并重启 Docker' : '已保存（未重启，下次启动 Docker 生效）')
    mirrorInfo.value.live_mirrors = d.live_mirrors
    mirrorDlg.value = false
  } finally {
    mirrorSaving.value = false
  }
}

async function load() {
  loading.value = true
  try {
    docker.value = await api.get('/apps/docker/status')
    templates.value = (await api.get('/apps/templates')) as any
    apps.value = (await api.get('/apps')) as any
    try {
      mirrors.value = (await api.get('/apps/mirrors')) as any
    } catch {
      mirrors.value = []
    }
  } finally {
    loading.value = false
  }
}

function openInstall(t: any) {
  cur.value = t
  instName.value = t.key
  instMirror.value = 'auto'
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
    await api.post('/apps', { app_key: cur.value.key, name: instName.value, mirror: instMirror.value, params: { ...instParams } }, { timeout: 900000 })
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
.wm-tpl-card__pills {
  display: flex;
  gap: 4px;
  align-items: center;
}
.hint {
  color: var(--wm-text-2, #8492a6);
  font-size: 12px;
  line-height: 1.7;
  margin-top: 4px;
}
</style>
