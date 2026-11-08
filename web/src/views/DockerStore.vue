<template>
  <div class="wm-page store-page">
    <WmSectionCard dense class="store-control">
      <div class="store-control__row">
        <div class="store-control__stats">
          <div class="store-stat"><span>Docker</span><b>{{ docker.installed ? (docker.compose ? '就绪' : '缺 compose') : '未安装' }}</b></div>
          <div class="store-stat"><span>容器模板</span><b>{{ templates.length }}</b></div>
          <div class="store-stat"><span>内置模板</span><b>{{ builtinCount }}</b></div>
          <div class="store-stat"><span>已部署</span><b>{{ apps.length }}</b></div>
        </div>
        <div class="store-control__actions">
          <a-input v-model:value="keyword" placeholder="搜索容器模板" allow-clear style="width: 200px">
            <template #prefix><SearchOutlined /></template>
          </a-input>
          <a-radio-group v-model:value="sourceFilter" size="small">
            <a-radio-button value="all">全部</a-radio-button>
            <a-radio-button value="builtin">内置</a-radio-button>
            <a-radio-button value="remote">镜像源</a-radio-button>
          </a-radio-group>
          <a-button size="small" @click="openMirrors">镜像源配置</a-button>
          <a-button size="small" @click="load"><ReloadOutlined />刷新</a-button>
        </div>
      </div>
    </WmSectionCard>

    <a-row :gutter="16" class="store-grid">
      <a-col :xs="24" :xl="16">
        <WmSectionCard title="容器模板" description="Docker Compose 编排，部署时可为镜像选择加速源">
          <template #actions>
            <a-button v-if="!docker.installed" size="small" data-tone="warning" :loading="installing" @click="installDocker">一键安装 Docker</a-button>
            <span class="wm-text-muted">显示 {{ filteredTemplates.length }} / {{ templates.length }}</span>
          </template>
          <a-row :gutter="[12, 12]" v-if="filteredTemplates.length">
            <a-col v-for="t in filteredTemplates" :key="t.key" :xs="24" :sm="12" :lg="8">
              <div class="store-card">
                <div class="store-card__head">
                  <span class="store-card__avatar store-card__avatar--docker">{{ (t.name || '?').slice(0, 1) }}</span>
                  <div class="store-card__title">
                    <b class="wm-truncate">{{ t.name }}</b>
                    <span v-if="t.version" class="store-card__ver">v{{ t.version }}</span>
                  </div>
                </div>
                <p class="store-card__desc">{{ t.desc }}</p>
                <div class="store-card__tags">
                  <WmStatusPill :label="t.source === 'remote' ? '镜像源' : '内置'" :tone="t.source === 'remote' ? 'danger' : 'success'" :dot="false" outline />
                </div>
                <div class="store-card__foot">
                  <a-button type="primary" size="small" :disabled="!docker.installed || !docker.compose" @click="openInstall(t)">部署</a-button>
                </div>
              </div>
            </a-col>
          </a-row>
          <WmEmptyState v-else compact title="没有匹配的模板" description="试试更换关键词，或把来源筛选切回「全部」。" />
        </WmSectionCard>

        <WmSectionCard title="实例明细" description="启动、停止、重启、查看日志与卸载" flush>
          <WmTable :data="apps" v-loading="loading" empty-title="还没有部署应用" empty-description="从上方模板中选择一个，填写实例名称即可部署。">
            <WmColumn label="实例名" min-width="140"><template #default="{ row }"><b>{{ row.app.name }}</b></template></WmColumn>
            <WmColumn label="应用" width="120"><template #default="{ row }"><span class="wm-mono">{{ row.app.app_key }}</span></template></WmColumn>
            <WmColumn label="状态" min-width="240"><template #default="{ row }"><span class="mono">{{ row.status }}</span></template></WmColumn>
            <WmColumn label="部署时间" width="170"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.app.created_at) }}</span></template></WmColumn>
            <WmColumn label="操作" width="310" fixed="right">
              <template #default="{ row }">
                <a-button size="small" data-tone="success" @click="act(row, 'start')">启动</a-button>
                <a-button size="small" data-tone="warning" @click="act(row, 'stop')">停止</a-button>
                <a-button size="small" @click="act(row, 'restart')">重启</a-button>
                <a-button size="small" @click="viewLogs(row)">日志</a-button>
                <a-button size="small" data-tone="danger" @click="uninstall(row)">卸载</a-button>
              </template>
            </WmColumn>
          </WmTable>
        </WmSectionCard>
      </a-col>

      <a-col :xs="24" :xl="8">
        <WmSectionCard title="已部署" :description="`${apps.length} 个实例`">
          <a-list v-if="apps.length" :data-source="apps" size="small" class="store-list">
            <template #renderItem="{ item }">
              <a-list-item class="store-list__item">
                <div class="store-list__meta">
                  <b class="wm-truncate">{{ item.app.name }}</b>
                  <span class="wm-text-muted wm-truncate">{{ item.app.app_key }}</span>
                </div>
                <template #actions>
                  <a-button type="link" size="small" @click="viewLogs(item)">日志</a-button>
                </template>
              </a-list-item>
            </template>
          </a-list>
          <WmEmptyState v-else compact title="还没有部署容器" description="从左侧卡片选择模板并点击部署。" />
        </WmSectionCard>

        <WmSectionCard title="运行环境" description="Docker 与镜像加速状态">
          <div class="store-env">
            <div><span>引擎</span><WmStatusPill :label="docker.installed ? '已安装' : '未安装'" :tone="docker.installed ? 'success' : 'danger'" /></div>
            <div><span>compose 插件</span><WmStatusPill :label="docker.compose ? '就绪' : '缺失'" :tone="docker.compose ? 'success' : 'warning'" /></div>
            <div><span>模板来源</span><b>内置 {{ builtinCount }} · 镜像源 {{ remoteCount }}</b></div>
          </div>
          <WmNotice tone="neutral" class="wm-mt-4">镜像加速写入 daemon.json，修改前自动备份；保存后需重启 Docker，运行中的容器会短暂中断。</WmNotice>
        </WmSectionCard>
      </a-col>
    </a-row>

    <a-modal v-model:open="dlg" :title="`部署 ${cur?.name || ''}`" width="560px">
      <a-form :label-col="{ style: { width: '130px' } }">
        <a-form-item label="实例名称"><a-input v-model:value="instName" placeholder="如 my-redis" /></a-form-item>
        <a-form-item label="镜像源">
          <a-select v-model:value="instMirror" style="width: 100%">
            <a-select-option v-for="m in dockerMirrors" :key="m.id" :value="m.id">{{ m.name }}</a-select-option>
          </a-select>
          <div class="hint">部署时将 YAML 中的镜像改写为所选代理仓库拉取；也可在「镜像源配置」里设置全局 daemon.json 加速</div>
        </a-form-item>
        <a-form-item v-for="p in cur?.params || []" :key="p.key" :label="p.label">
          <a-input v-model:value="instParams[p.key]" :placeholder="p.def" :type="p.key.includes('pass') || p.key === 'password' ? 'password' : 'text'" />
        </a-form-item>
      </a-form>
      <template #footer>
        <a-button @click="dlg = false">取消</a-button>
        <a-button type="primary" :loading="deploying" @click="deploy">部署（拉取镜像可能较慢）</a-button>
      </template>
    </a-modal>

    <a-modal v-model:open="logDlg" :title="`日志 - ${logName}`" width="820px">
      <WmCodeBlock :content="logContent" :label="`${logName} / logs`" />
    </a-modal>

    <a-modal v-model:open="mirrorDlg" title="Docker 镜像源配置（daemon.json）" width="680px">
      <WmNotice v-if="mirrorInfo.docker_installed === false" tone="warning" title="尚未检测到 Docker" class="wm-mb-4">可先编辑配置文件，安装 Docker 后生效。</WmNotice>
      <a-form :label-col="{ style: { width: '110px' } }">
        <a-form-item label="加速镜像源">
          <a-textarea v-model:value="mirrorForm.registry_mirrors" :rows="5" placeholder="每行一个，如 https://docker.m.daocloud.io" />
          <div class="presets">
            <span class="hint">常用源（时效性强，按可用性自行取舍）：</span>
            <a-tag v-for="m in mirrorPresets" :key="m" size="small" class="preset-tag" @click="addPreset(m)">{{ m }}</a-tag>
          </div>
        </a-form-item>
        <a-form-item label="私有仓库白名单">
          <a-textarea v-model:value="mirrorForm.insecure_registries" :rows="3" placeholder="每行一个 HTTP 私有仓库，如 192.168.1.10:5000" />
        </a-form-item>
        <a-form-item v-if="mirrorInfo.live_mirrors && mirrorInfo.live_mirrors.length" label="当前生效">
          <div class="live-mirrors">
            <WmStatusPill v-for="m in mirrorInfo.live_mirrors" :key="m" :label="m" tone="success" :dot="false" mono />
          </div>
        </a-form-item>
        <a-form-item label="配置文件"><span class="hint wm-mono">{{ mirrorInfo.path }}</span><span class="hint">其余配置项保持不变，修改前自动备份</span></a-form-item>
      </a-form>
      <template #footer>
        <a-button @click="mirrorDlg = false">取消</a-button>
        <a-button type="primary" :loading="mirrorSaving" @click="saveMirrors">保存并重启 Docker</a-button>
      </template>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { SearchOutlined, ReloadOutlined } from '@ant-design/icons-vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from '../antd-compat'
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
const instParams = reactive<Record<string, string>>({})
const logDlg = ref(false)
const logName = ref('')
const logContent = ref('')

const builtinCount = computed(() => templates.value.filter((t) => t.source !== 'remote').length)
const remoteCount = computed(() => templates.value.filter((t) => t.source === 'remote').length)

const keyword = ref('')
const sourceFilter = ref<'all' | 'builtin' | 'remote'>('all')
const filteredTemplates = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  return templates.value.filter((t) => {
    if (sourceFilter.value === 'builtin' && t.source === 'remote') return false
    if (sourceFilter.value === 'remote' && t.source !== 'remote') return false
    if (!q) return true
    return [t.name, t.key, t.desc].some((v: any) => String(v || '').toLowerCase().includes(q))
  })
})

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

const dockerMirrors = computed(() => [
  { id: 'auto', name: '自动（优先加速源）' },
  { id: 'off', name: '官方直连' },
  ...mirrorsList.value.filter((m: any) => m.docker)
])
const mirrorsList = ref<any[]>([])

async function load() {
  loading.value = true
  try {
    docker.value = await api.get('/apps/docker/status')
    templates.value = ((await api.get('/apps/templates')) as any).filter((t: any) => t.kind === 'compose')
    apps.value = ((await api.get('/apps')) as any).filter((a: any) => a.app.kind !== 'native')
    try {
      mirrorsList.value = (await api.get('/apps/mirrors')) as any
    } catch {
      mirrorsList.value = []
    }
  } finally {
    loading.value = false
  }
}

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
.mono {
  font-family: Consolas, Monaco, monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-all;
}
.hint {
  color: var(--wm-text-2, #8492a6);
  font-size: 12px;
  line-height: 1.7;
  margin-top: 4px;
}
.presets {
  margin-top: 8px;
  line-height: 2;
}
.preset-tag {
  cursor: pointer;
  margin-right: 6px;
}
.live-mirrors {
  line-height: 1.8;
}
</style>
