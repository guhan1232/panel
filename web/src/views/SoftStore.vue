<template>
  <div class="wm-page store-page">
    <!-- Control strip: search, source filter, quick counters -->
    <WmSectionCard dense class="store-control">
      <div class="store-control__row">
        <div class="store-control__stats">
          <div class="store-stat"><span>可安装软件</span><b>{{ templates.length }}</b></div>
          <div class="store-stat"><span>内置模板</span><b>{{ builtinCount }}</b></div>
          <div class="store-stat"><span>镜像源</span><b>{{ remoteCount }}</b></div>
          <div class="store-stat"><span>已安装实例</span><b>{{ apps.length }}</b></div>
        </div>
        <div class="store-control__actions">
          <a-input v-model:value="keyword" placeholder="搜索软件或运行时" allow-clear style="width: 220px">
            <template #prefix><SearchOutlined /></template>
          </a-input>
          <a-radio-group v-model:value="sourceFilter" size="small">
            <a-radio-button value="all">全部</a-radio-button>
            <a-radio-button value="builtin">内置</a-radio-button>
            <a-radio-button value="remote">镜像源</a-radio-button>
          </a-radio-group>
          <a-button size="small" @click="load"><ReloadOutlined />刷新</a-button>
        </div>
      </div>
    </WmSectionCard>

    <a-row :gutter="16" class="store-grid">
      <a-col :xs="24" :xl="16">
        <WmSectionCard title="可安装软件" description="原生二进制与系统包一键部署，下载源可在安装时选择">
          <template #actions><span class="wm-text-muted">显示 {{ filteredTemplates.length }} / {{ templates.length }}</span></template>
          <a-row :gutter="[12, 12]" v-if="filteredTemplates.length">
            <a-col v-for="t in filteredTemplates" :key="t.key" :xs="24" :sm="12" :lg="8">
              <div class="store-card">
                <div class="store-card__head">
                  <span class="store-card__avatar">{{ (t.name || '?').slice(0, 1) }}</span>
                  <div class="store-card__title">
                    <b class="wm-truncate">{{ t.name }}</b>
                    <span v-if="t.version" class="store-card__ver">v{{ t.version }}</span>
                  </div>
                </div>
                <p class="store-card__desc">{{ t.desc }}</p>
                <div class="store-card__tags">
                  <WmStatusPill :label="kindLabel(t.kind)" :tone="t.kind === 'package' ? 'success' : t.kind === 'native' ? 'warning' : 'info'" :dot="false" outline />
                  <WmStatusPill :label="t.source === 'remote' ? '镜像源' : '内置'" :tone="t.source === 'remote' ? 'danger' : 'success'" :dot="false" outline />
                </div>
                <div class="store-card__foot">
                  <a-button type="primary" size="small" @click="openInstall(t)">安装</a-button>
                </div>
              </div>
            </a-col>
          </a-row>
          <WmEmptyState v-else compact title="没有匹配的软件" description="试试更换关键词，或把来源筛选切回「全部」。" />
        </WmSectionCard>

        <WmSectionCard title="运行时环境" description="编程语言多版本共存，安装后软链到 /usr/local/bin">
          <template #actions><span class="wm-text-muted">{{ filteredRuntimes.length }} 个运行时</span></template>
          <a-row :gutter="[12, 12]" v-if="filteredRuntimes.length">
            <a-col v-for="r in filteredRuntimes" :key="r.key" :xs="24" :sm="12" :lg="8">
              <div class="store-card store-card--compact">
                <div class="store-card__head">
                  <span class="store-card__avatar store-card__avatar--runtime">{{ (r.name || '?').slice(0, 1) }}</span>
                  <div class="store-card__title">
                    <b class="wm-truncate">{{ r.name }}</b>
                    <WmStatusPill :label="`${r.installed.length} 个版本`" tone="info" :dot="false" />
                  </div>
                </div>
                <p class="store-card__desc">{{ r.desc }}</p>
                <div class="rt-chips" v-if="r.installed.length">
                  <a-tag v-for="i in r.installed" :key="i.id" :data-tone="i.is_default ? 'success' : 'primary'">{{ i.version }}{{ i.is_default ? '（默认）' : '' }}</a-tag>
                </div>
                <div class="store-card__foot">
                  <a-button size="small" @click="openRt(r)">安装 / 管理版本</a-button>
                </div>
              </div>
            </a-col>
          </a-row>
          <WmEmptyState v-else compact title="没有匹配的运行时" description="更换关键词后再试。" />
        </WmSectionCard>
      </a-col>

      <a-col :xs="24" :xl="8">
        <WmSectionCard title="已安装" :description="`${apps.length} 个实例，可查看日志或卸载`">
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
          <WmEmptyState v-else compact title="还没有安装软件" description="从左侧卡片选择软件并点击安装。" />
        </WmSectionCard>

        <WmSectionCard title="下载源" description="安装时可为发行包选择来源">
          <a-list :data-source="sources" size="small" class="store-list">
            <template #renderItem="{ item }">
              <a-list-item class="store-list__item">
                <div class="store-list__meta"><b class="wm-truncate">{{ item.name }}</b></div>
              </a-list-item>
            </template>
          </a-list>
          <WmNotice tone="neutral" class="wm-mt-4">镜像站仅收录部分软件，未收录的源会在安装界面置灰；GitHub 类发布走加速通道。</WmNotice>
        </WmSectionCard>
      </a-col>
    </a-row>

    <WmSectionCard title="实例明细" description="启动、停止、重启、查看日志与卸载" flush>
      <WmTable :data="apps" v-loading="loading" empty-title="还没有安装软件" empty-description="从上方选择软件，选择下载源即可安装。">
        <WmColumn label="实例名" min-width="140"><template #default="{ row }"><b>{{ row.app.name }}</b></template></WmColumn>
        <WmColumn label="软件" width="130"><template #default="{ row }"><span class="wm-mono">{{ row.app.app_key }}</span></template></WmColumn>
        <WmColumn label="形态" width="90">
          <template #default="{ row }"><WmStatusPill :label="kindLabel(row.app.kind)" :tone="row.app.kind === 'package' ? 'success' : row.app.kind === 'native' ? 'warning' : 'info'" :dot="false" outline /></template>
        </WmColumn>
        <WmColumn label="状态" min-width="200"><template #default="{ row }"><span class="mono">{{ row.status }}</span></template></WmColumn>
        <WmColumn label="安装时间" width="170"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.app.created_at) }}</span></template></WmColumn>
        <WmColumn label="操作" width="310" fixed="right">
          <template #default="{ row }">
            <template v-if="!row.app.tool">
              <a-button size="small" data-tone="success" @click="act(row, 'start')">启动</a-button>
              <a-button size="small" data-tone="warning" @click="act(row, 'stop')">停止</a-button>
              <a-button size="small" @click="act(row, 'restart')">重启</a-button>
            </template>
            <a-button size="small" @click="viewLogs(row)">日志</a-button>
            <a-button size="small" data-tone="danger" @click="uninstall(row)">卸载</a-button>
          </template>
        </WmColumn>
      </WmTable>
    </WmSectionCard>

    <a-drawer v-model:open="rtDlg" width="62%" :title="`运行时版本 - ${rtLang?.name || ''}`">
      <WmToolbar>
        <a-select v-model:value="rtSource" style="width: 190px" @change="loadVersions">
          <a-select-option v-for="s in rtLang?.sources || []" :key="s.id" :value="s.id">{{ s.name }}</a-select-option>
        </a-select>
        <a-select v-model:value="rtFilterMode" style="width: 150px">
          <a-select-option value="common">显示常用版本</a-select-option>
          <a-select-option value="lts">只显示LTS版本</a-select-option>
          <a-select-option value="all">显示所有版本</a-select-option>
        </a-select>
        <a-input v-model:value="rtFilter" placeholder="版本过滤" style="width: 140px" allow-clear />
        <a-button size="small" @click="loadVersions">更新版本列表</a-button>
        <template #note>共 {{ filteredVersions.length }} 个</template>
        <template v-if="rtLang?.key === 'nodejs'">
          <span class="hint">registry源</span>
          <a-select v-model:value="npmRegistry" placeholder="未设置" style="width: 200px" :loading="regLoading" @change="saveRegistry">
            <a-select-option v-for="o in registryOptions" :key="o.id" :value="o.id">{{ o.name }}</a-select-option>
          </a-select>
        </template>
        <template v-if="rtLang?.key === 'python'">
          <span class="hint">pypi源</span>
          <a-select v-model:value="pypiIndex" placeholder="未设置" style="width: 240px" :loading="pypiLoading" @change="savePypi">
            <a-select-option v-for="o in pypiOptions" :key="o.id" :value="o.id">{{ o.name }}</a-select-option>
          </a-select>
        </template>
        <template v-if="rtLang?.key === 'php'">
          <span class="hint">Composer源</span>
          <a-select v-model:value="composerUrl" placeholder="未设置" style="width: 210px" :loading="composerLoading" @change="saveComposer">
            <a-select-option v-for="o in composerMirrorOptions" :key="o.id" :value="o.id">{{ o.name }}</a-select-option>
          </a-select>
          <a-button v-if="!composerInstalled" size="small" type="primary" :loading="composerInstalling" @click="installComposer">安装 Composer</a-button>
          <span class="hint">PECL源</span>
          <a-select v-model:value="peclMirror" placeholder="未设置" style="width: 190px" @change="savePecl">
            <a-select-option v-for="o in peclOptions" :key="o.id" :value="o.id">{{ o.name }}</a-select-option>
          </a-select>
        </template>
        <template v-if="rtInstalled.length && rtLang?.key === 'nodejs'">
          <span class="hint">命令行版本</span>
          <a-select v-model:value="defaultInstId" style="width: 160px" @change="setDefaultById">
            <a-select-option v-for="i in rtInstalled" :key="i.id" :value="i.id">{{ i.version }}</a-select-option>
          </a-select>
        </template>
      </WmToolbar>
      <WmTable :data="filteredVersions" v-loading="rtLoading" size="small" max-height="380">
        <WmColumn label="版本" width="150">
          <template #default="{ row }"><b>{{ row.version }}</b> <a-tag v-if="row.lts" size="small" data-tone="success">LTS</a-tag></template>
        </WmColumn>
        <WmColumn v-if="rtLang?.key === 'nodejs'" label="NPM" width="90"><template #default="{ row }">{{ row.npm || '-' }}</template></WmColumn>
        <WmColumn v-if="rtLang?.key === 'nodejs'" label="V8" width="110"><template #default="{ row }">{{ row.v8 || '-' }}</template></WmColumn>
        <WmColumn label="发布日期" width="120"><template #default="{ row }">{{ row.date || '-' }}</template></WmColumn>
        <WmColumn label="操作" width="110">
          <template #default="{ row }"><a-button size="small" type="primary" :loading="installingVer === row.version" @click="installVer(row)">安装</a-button></template>
        </WmColumn>
      </WmTable>
      <h4 class="store-subhead">已安装</h4>
      <WmTable :data="rtInstalled" size="small">
        <WmColumn prop="version" label="版本" width="150" />
        <WmColumn label="默认" width="70"><template #default="{ row }"><a-tag v-if="row.is_default" size="small" data-tone="success">默认</a-tag><span v-else>-</span></template></WmColumn>
        <WmColumn label="软件路径" min-width="220"><template #default="{ row }"><span class="mono">{{ row.dir }}</span></template></WmColumn>
        <WmColumn label="安装时间" width="165"><template #default="{ row }">{{ fmtTime(row.created_at) }}</template></WmColumn>
        <WmColumn label="操作" width="180">
          <template #default="{ row }">
            <a-button size="small" :disabled="row.is_default || rtLang?.key === 'php'" @click="setDefault(row)">设为默认</a-button>
            <a-button size="small" data-tone="danger" @click="rmRuntime(row)">卸载</a-button>
          </template>
        </WmColumn>
      </WmTable>
    </a-drawer>

    <a-modal v-model:open="dlg" :title="`安装 ${cur?.name || ''}`" width="560px">
      <a-form :label-col="{ style: { width: '110px' } }">
        <a-form-item label="实例名称"><a-input v-model:value="instName" placeholder="如 prod-node" /></a-form-item>
        <a-form-item v-if="cur?.versions?.length" label="版本">
          <a-select v-model:value="instVer" style="width: 100%">
            <a-select-option v-for="v in cur.versions" :key="v.version" :value="v.version">
              {{ v.version }}<template v-if="v.note">（{{ v.note }}）</template>
            </a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="下载源">
          <a-select v-model:value="instSource" style="width: 100%">
            <a-select-option v-for="s in sources" :key="s.id"
              :value="s.id" :disabled="cur && cur.soft_sources && !cur.soft_sources.includes(s.id)">{{ s.name }}</a-select-option>
          </a-select>
          <div class="hint">{{ cur?.kind === 'package' ? '系统包经所选镜像站的 apt 仓库定向安装（临时源，不改动系统全局配置）；未被收录的仓库自动回退官方源' : '镜像站仅收录部分软件（未收录的源会置灰）；GitHub 类发布走加速通道' }}</div>
        </a-form-item>
        <a-form-item v-if="cur?.kind === 'package'" label="安装方式">
          <div class="hint">通过系统包管理器（apt/dnf）安装，由 systemd 托管</div>
        </a-form-item>
        <a-form-item v-for="p in cur?.params || []" :key="p.key" :label="p.label">
          <a-input v-model:value="instParams[p.key]" :placeholder="p.def" :type="p.key.includes('pass') || p.key === 'password' ? 'password' : 'text'" />
        </a-form-item>
      </a-form>
      <template #footer>
        <a-button @click="dlg = false">取消</a-button>
        <a-button type="primary" :loading="deploying" @click="deploy">安装（下载发行包可能较慢）</a-button>
      </template>
    </a-modal>

    <a-modal v-model:open="logDlg" :title="`日志 - ${logName}`" width="820px">
      <WmCodeBlock :content="logContent" :label="`${logName} / logs`" />
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { SearchOutlined, ReloadOutlined } from '@ant-design/icons-vue'
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from '../antd-compat'
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
const instVer = ref('')
const instParams = reactive<Record<string, string>>({})
const logDlg = ref(false)
const logName = ref('')
const logContent = ref('')

const kindLabel = (k: string) => ({ native: '二进制', package: '系统包', compose: '容器' } as any)[k] || k
const builtinCount = computed(() => templates.value.filter((t) => t.source !== 'remote').length)

const runtimes = ref<any[]>([])
const rtDlg = ref(false)
const rtLang = ref<any>(null)
const rtSource = ref('')
const rtVersions = ref<any[]>([])
const rtFilter = ref('')
const rtLoading = ref(false)
const installingVer = ref('')
const rtFilterMode = ref('common')
const npmRegistry = ref('')
const regLoading = ref(false)
const registryOptions = [
  { id: 'https://registry.npmmirror.com', name: 'npmmirror 中国镜像' },
  { id: 'https://registry.npmjs.org', name: 'npm 官方' },
  { id: 'https://mirrors.cloud.tencent.com/npm/', name: '腾讯云镜像' },
  { id: 'https://mirrors.huaweicloud.com/repository/npm/', name: '华为云镜像' }
]
const pypiOptions = [
  { id: 'https://pypi.tuna.tsinghua.edu.cn/simple/', name: '清华大学镜像（社区首选）' },
  { id: 'https://mirrors.aliyun.com/pypi/simple/', name: '阿里云镜像' },
  { id: 'https://pypi.mirrors.ustc.edu.cn/simple/', name: '中科大镜像' },
  { id: 'https://repo.huaweicloud.com/repository/pypi/simple/', name: '华为云镜像' },
  { id: 'https://mirrors.cloud.tencent.com/pypi/simple/', name: '腾讯云镜像' },
  { id: 'https://pypi.doubanio.com/simple/', name: '豆瓣镜像' },
  { id: 'https://pypi.org/simple/', name: 'PyPI 官方' }
]
const pypiIndex = ref('')
const pypiLoading = ref(false)
const composerUrl = ref('')
const composerLoading = ref(false)
const composerInstalling = ref(false)
const composerInstalled = ref(false)
const peclMirror = ref('')
const composerMirrorOptions = [
  { id: 'https://mirrors.aliyun.com/composer/', name: '阿里云镜像（首选）' },
  { id: 'https://mirrors.cloud.tencent.com/composer/', name: '腾讯云镜像' },
  { id: 'https://repo.huaweicloud.com/repository/php/', name: '华为云镜像' },
  { id: 'https://packagist.cn', name: 'Packagist CN' },
  { id: 'https://packagist.org', name: 'Packagist 官方' }
]
const peclOptions = [
  { id: 'https://pecl.php.net', name: 'PECL 官方' },
  { id: 'https://mirrors.aliyun.com/pecl/', name: '阿里云镜像' },
  { id: 'https://mirrors.tuna.tsinghua.edu.cn/pecl/', name: '清华 TUNA' }
]
const defaultInstId = computed(() => {
  const d = rtInstalled.value.find((i) => i.is_default)
  return d ? d.id : undefined
})
const rtInstalled = computed(() => rtLang.value?.installed || [])
const filteredVersions = computed(() => {
  let list = rtVersions.value
  if (rtFilterMode.value === 'lts') list = list.filter((v) => v.lts)
  else if (rtFilterMode.value === 'common') {
    const newest = list[0]?.version
    list = list.filter((v) => v.lts || v.version === newest)
  }
  const f = rtFilter.value.trim().toLowerCase()
  if (!f) return list
  return list.filter((v) => v.version.toLowerCase().includes(f))
})

async function loadRuntimes() {
  try {
    runtimes.value = (await api.get('/runtimes')) as any
  } catch {
    runtimes.value = []
  }
}

function openRt(r: any) {
  rtLang.value = r
  rtSource.value = r.sources?.[0]?.id || ''
  rtVersions.value = []
  rtFilter.value = ''
  rtFilterMode.value = r.key === 'nodejs' ? 'common' : 'all'
  npmRegistry.value = ''
  rtDlg.value = true
  loadVersions()
  if (r.key === 'nodejs') loadRegistry()
  if (r.key === 'python') loadPypi()
  if (r.key === 'php') { loadComposerInfo(); loadPecl() }
}

async function loadRegistry() {
  regLoading.value = true
  try {
    const d: any = await api.get('/runtimes/nodejs/registry')
    npmRegistry.value = d.registry || ''
  } finally {
    regLoading.value = false
  }
}

async function saveRegistry(val: string) {
  await api.post('/runtimes/nodejs/registry', { registry: val })
  ElMessage.success('npm registry 已设置')
}

async function loadPypi() {
  pypiLoading.value = true
  try {
    const d: any = await api.get('/runtimes/python/pypi')
    pypiIndex.value = d.index_url || ''
  } finally {
    pypiLoading.value = false
  }
}

async function savePypi(val: string) {
  await api.post('/runtimes/python/pypi', { index_url: val })
  ElMessage.success('pip 源已设置，pip install 将走该镜像')
}

async function setDefaultById(id: number) {
  await setDefault({ id })
}

async function loadVersions() {
  if (!rtLang.value) return
  rtLoading.value = true
  try {
    rtVersions.value = (await api.get(`/runtimes/${rtLang.value.key}/versions`, { params: { source: rtSource.value }, timeout: 60000 })) as any
  } finally {
    rtLoading.value = false
  }
}

async function installVer(row: any) {
  installingVer.value = row.version
  try {
    const d: any = await api.post(`/runtimes/${rtLang.value.key}/install`, { version: row.version, source: rtSource.value }, { timeout: 900000 })
    ElMessage.success('安装完成，软链: ' + (d.links || []).join(', '))
    loadRuntimes()
  } finally {
    installingVer.value = ''
  }
}

async function setDefault(row: any) {
  await api.post(`/runtimes/${rtLang.value.key}/set-default`, { instance_id: row.id })
  ElMessage.success('已设为默认')
  loadRuntimes()
  if (rtDlg.value && rtLang.value) {
    const cur = runtimes.value.find((r) => r.key === rtLang.value.key)
    if (cur) rtLang.value = cur
  }
}

async function rmRuntime(row: any) {
  try {
    await ElMessageBox.confirm(`确认卸载 ${rtLang.value?.name || ''} ${row.version}？对应软链将移除。`, '卸载运行时', { type: 'warning' })
  } catch {
    return
  }
  await api.delete(`/runtimes/${row.id}`)
  ElMessage.success('已卸载')
  loadRuntimes()
}
const remoteCount = computed(() => templates.value.filter((t) => t.source === 'remote').length)

const keyword = ref('')
const sourceFilter = ref<'all' | 'builtin' | 'remote'>('all')
const filteredTemplates = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  return templates.value.filter((t) => {
    if (sourceFilter.value === 'builtin' && t.source === 'remote') return false
    if (sourceFilter.value === 'remote' && t.source !== 'remote') return false
    if (!q) return true
    return [t.name, t.key, t.desc, kindLabel(t.kind)].some((v: any) => String(v || '').toLowerCase().includes(q))
  })
})
const filteredRuntimes = computed(() => {
  const q = keyword.value.trim().toLowerCase()
  if (!q) return runtimes.value
  return runtimes.value.filter((r) => [r.name, r.key, r.desc].some((v: any) => String(v || '').toLowerCase().includes(q)))
})

async function load() {
  loading.value = true
  try {
    loadRuntimes()
    templates.value = ((await api.get('/apps/templates')) as any).filter((t: any) => t.kind === 'native' || t.kind === 'package')
    apps.value = ((await api.get('/apps')) as any).filter((a: any) => a.app.kind === 'native' || a.app.kind === 'package')
    sources.value = (await api.get('/apps/soft-sources')) as any
  } finally {
    loading.value = false
  }
}

async function openInstall(t: any) {
  cur.value = t
  instName.value = t.key
  instSource.value = 'auto'
  instVer.value = (t as any).default_version || ''
  Object.keys(instParams).forEach((k) => delete instParams[k])
  for (const p of t.params || []) instParams[p.key] = p.def || ''
  dlg.value = true
  // native/package 拉取可选版本清单
  if (t.kind === 'native' || t.kind === 'package') {
    try {
      const d: any = await api.get('/apps/versions', { params: { key: t.key } })
      cur.value = { ...t, versions: d.versions || [] }
      if (d.default && !instVer.value) instVer.value = d.default
    } catch { /* 拉取失败时无版本选择 */ }
  }
}

async function deploy() {
  if (!instName.value) {
    ElMessage.warning('请填写实例名称')
    return
  }
  deploying.value = true
  try {
    const d: any = await api.post('/apps', { app_key: cur.value.key, name: instName.value, version: instVer.value, mirror: instSource.value, params: { ...instParams } }, { timeout: 900000 })
    ElMessage.success(d?.status || '安装完成')
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

<style scoped>
.rt-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: 8px;
}
</style>

async function loadComposerInfo() {
  composerLoading.value = true
  try {
    const d: any = await api.get('/runtimes/php/composer')
    composerInstalled.value = !!d.installed
    composerUrl.value = d.url || ''
  } finally {
    composerLoading.value = false
  }
}

async function saveComposer(val: string) {
  await api.post('/runtimes/php/composer', { url: val })
  ElMessage.success('Composer 源已设置')
}

async function installComposer() {
  composerInstalling.value = true
  try {
    await api.post('/runtimes/php/composer/install', {}, { timeout: 900000 })
    ElMessage.success('Composer 已安装')
    composerInstalled.value = true
  } finally {
    composerInstalling.value = false
  }
}

async function loadPecl() {
  try {
    const d: any = await api.get('/runtimes/php/pecl')
    peclMirror.value = d.mirror || ''
  } catch {
    peclMirror.value = ''
  }
}

async function savePecl(val: string) {
  await api.post('/runtimes/php/pecl', { mirror: val })
  ElMessage.success('PECL 镜像已设置')
}