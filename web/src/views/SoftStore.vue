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
                <WmStatusPill :label="kindLabel(t.kind)" :tone="t.kind === 'package' ? 'success' : t.kind === 'native' ? 'warning' : 'info'" :dot="false" outline />
                <WmStatusPill :label="t.source === 'remote' ? '镜像源' : '内置'" :tone="t.source === 'remote' ? 'danger' : 'success'" :dot="false" outline />
              </div>
            </div>
            <p class="wm-tpl-card__desc">{{ t.desc }}<template v-if="t.version"> · v{{ t.version }}</template></p>
            <el-button size="small" type="primary" class="wm-tpl-card__btn" @click="openInstall(t)">安装</el-button>
          </article>
        </div>
        <WmEmptyState v-else compact title="加载中" description="正在读取软件清单。" />
      </div>

      <div class="wm-section-block">
        <div class="wm-section-block__head">
          <span class="wm-eyebrow">运行时环境</span>
          <span class="wm-text-muted">编程语言多版本共存，安装后软链 /usr/local/bin</span>
        </div>
        <div class="wm-tpl-grid">
          <article v-for="r in runtimes" :key="r.key" class="wm-tpl-card">
            <div class="wm-tpl-card__top">
              <h3 class="wm-tpl-card__name">{{ r.name }}</h3>
              <WmStatusPill :label="`${r.installed.length} 个版本`" tone="info" :dot="false" />
            </div>
            <p class="wm-tpl-card__desc">{{ r.desc }}</p>
            <div class="rt-chips" v-if="r.installed.length">
              <el-tag v-for="i in r.installed" :key="i.id" size="small" :type="i.is_default ? 'success' : 'info'">{{ i.version }}{{ i.is_default ? ' · 默认' : '' }}</el-tag>
            </div>
            <el-button size="small" type="primary" class="wm-tpl-card__btn" @click="openRt(r)">安装 / 管理版本</el-button>
          </article>
        </div>
      </div>

      <el-drawer v-model="rtDlg" size="62%" :title="`运行时版本 - ${rtLang?.name || ''}`">
        <div class="toolbar">
          <el-select v-model="rtSource" style="width: 190px" @change="loadVersions">
            <el-option v-for="s in rtLang?.sources || []" :key="s.id" :label="s.name" :value="s.id" />
          </el-select>
          <el-select v-model="rtFilterMode" style="width: 150px">
            <el-option label="显示常用版本" value="common" />
            <el-option label="只显示LTS版本" value="lts" />
            <el-option label="显示所有版本" value="all" />
          </el-select>
          <el-input v-model="rtFilter" placeholder="版本过滤" style="width: 140px" clearable />
          <el-button size="small" @click="loadVersions">更新版本列表</el-button>
          <span class="hint">共 {{ filteredVersions.length }} 个</span>
          <template v-if="rtLang?.key === 'nodejs'">
            <span class="hint">registry源</span>
            <el-select v-model="npmRegistry" placeholder="未设置" style="width: 200px" :loading="regLoading" @change="saveRegistry">
              <el-option v-for="o in registryOptions" :key="o.id" :label="o.name" :value="o.id" />
            </el-select>
          </template>
          <template v-if="rtLang?.key === 'python'">
            <span class="hint">pypi源</span>
            <el-select v-model="pypiIndex" placeholder="未设置" style="width: 240px" :loading="pypiLoading" @change="savePypi">
              <el-option v-for="o in pypiOptions" :key="o.id" :label="o.name" :value="o.id" />
            </el-select>
          </template>
          <template v-if="rtLang?.key === 'php'">
            <span class="hint">Composer源</span>
            <el-select v-model="composerUrl" placeholder="未设置" style="width: 210px" :loading="composerLoading" @change="saveComposer">
              <el-option v-for="o in composerMirrorOptions" :key="o.id" :label="o.name" :value="o.id" />
            </el-select>
            <el-button v-if="!composerInstalled" size="small" type="primary" plain :loading="composerInstalling" @click="installComposer">安装 Composer</el-button>
            <span class="hint">PECL源</span>
            <el-select v-model="peclMirror" placeholder="未设置" style="width: 190px" @change="savePecl">
              <el-option v-for="o in peclOptions" :key="o.id" :label="o.name" :value="o.id" />
            </el-select>
          </template>
          <template v-if="rtInstalled.length && rtLang?.key === 'nodejs'">
            <span class="hint">命令行版本</span>
            <el-select v-model="defaultInstId" style="width: 160px" @change="setDefaultById">
              <el-option v-for="i in rtInstalled" :key="i.id" :label="i.version" :value="i.id" />
            </el-select>
          </template>
        </div>
        <el-table :data="filteredVersions" v-loading="rtLoading" size="small" max-height="380">
          <el-table-column label="版本" width="150">
            <template #default="{ row }"><b>{{ row.version }}</b> <el-tag v-if="row.lts" size="small" type="success">LTS</el-tag></template>
          </el-table-column>
          <el-table-column v-if="rtLang?.key === 'nodejs'" label="NPM" width="90"><template #default="{ row }">{{ row.npm || '-' }}</template></el-table-column>
          <el-table-column v-if="rtLang?.key === 'nodejs'" label="V8" width="110"><template #default="{ row }">{{ row.v8 || '-' }}</template></el-table-column>
          <el-table-column label="发布日期" width="120"><template #default="{ row }">{{ row.date || '-' }}</template></el-table-column>
          <el-table-column label="操作" width="110">
            <template #default="{ row }"><el-button size="small" type="primary" :loading="installingVer === row.version" @click="installVer(row)">安装</el-button></template>
          </el-table-column>
        </el-table>
        <h4 style="margin: 16px 0 8px">已安装</h4>
        <el-table :data="rtInstalled" size="small">
          <el-table-column prop="version" label="版本" width="150" />
          <el-table-column label="默认" width="70"><template #default="{ row }"><el-tag v-if="row.is_default" size="small" type="success">默认</el-tag><span v-else>-</span></template></el-table-column>
          <el-table-column label="软件路径" min-width="220"><template #default="{ row }"><span class="mono">{{ row.dir }}</span></template></el-table-column>
          <el-table-column label="安装时间" width="165"><template #default="{ row }">{{ fmtTime(row.created_at) }}</template></el-table-column>
          <el-table-column label="操作" width="180">
            <template #default="{ row }">
              <el-button size="small" :disabled="row.is_default || rtLang?.key === 'php'" @click="setDefault(row)">设为默认</el-button>
              <el-button size="small" type="danger" @click="rmRuntime(row)">卸载</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-drawer>

      <div class="wm-section-block wm-section-block--table">
        <div class="wm-section-block__head">
          <span class="wm-eyebrow">已安装软件</span>
          <span class="wm-text-muted">{{ apps.length }} 个实例</span>
        </div>
        <WmTable :data="apps" v-loading="loading" empty-title="还没有安装软件" empty-description="从上方选择软件，选择下载源即可安装。">
          <el-table-column label="实例名" min-width="140"><template #default="{ row }"><b>{{ row.app.name }}</b></template></el-table-column>
          <el-table-column label="软件" width="130"><template #default="{ row }"><span class="wm-mono">{{ row.app.app_key }}</span></template></el-table-column>
          <el-table-column label="形态" width="90">
            <template #default="{ row }"><WmStatusPill :label="kindLabel(row.app.kind)" :tone="row.app.kind === 'package' ? 'success' : row.app.kind === 'native' ? 'warning' : 'info'" :dot="false" outline /></template>
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
        <el-form-item v-if="cur?.kind !== 'package'" label="下载源">
          <el-select v-model="instSource" style="width: 100%">
            <el-option v-for="s in sources" :key="s.id" :label="s.name"
              :value="s.id" :disabled="cur && cur.soft_sources && !cur.soft_sources.includes(s.id)" />
          </el-select>
          <div class="hint">镜像站仅收录部分软件（未收录的源会置灰）；GitHub 类发布走加速通道</div>
        </el-form-item>
        <el-form-item v-if="cur?.kind === 'package'" label="安装方式">
          <div class="hint">通过系统包管理器（apt/dnf）安装，由 systemd 托管</div>
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