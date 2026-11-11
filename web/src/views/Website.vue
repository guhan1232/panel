<template>
  <div class="wm-page">
    <WmSectionCard title="站点列表" description="Nginx 配置在写入前执行 nginx -t 校验，失败自动回滚" flush>
      <WmToolbar>
        <WmStatusPill :label="nginx.installed ? 'Nginx 已安装' : 'Nginx 未安装'" :tone="nginx.installed ? 'success' : 'danger'" :pulse="!nginx.installed" />
        <span v-if="nginx.installed && nginx.version" class="wm-mono wm-text-muted">{{ nginx.version }}</span>
        <a-button v-if="!nginx.installed" size="small" data-tone="warning" :loading="installing" @click="installNginx">安装 Nginx</a-button>
        <a-button size="small" @click="reloadNginx">重载 Nginx</a-button>
        <a-button type="primary" size="small" @click="openCreate">创建站点</a-button>
        <a-button size="small" @click="load">刷新</a-button>
      </WmToolbar>

      <!-- 类型 Tab（宝塔式分类） -->
      <div class="type-tabs">
        <a-radio-group v-model:value="typeFilter" size="small" button-style="solid">
          <a-radio-button value="all">全部</a-radio-button>
          <a-radio-button value="html">HTML项目</a-radio-button>
          <a-radio-button value="php">PHP项目</a-radio-button>
          <a-radio-button value="proxy">反向代理</a-radio-button>
          <a-radio-button value="java">Java项目</a-radio-button>
          <a-radio-button value="node">Node项目</a-radio-button>
          <a-radio-button value="go">Go项目</a-radio-button>
          <a-radio-button value="python">Python项目</a-radio-button>
          <a-radio-button value="dotnet">.Net项目</a-radio-button>
          <a-radio-button value="other">其他项目</a-radio-button>
        </a-radio-group>
      </div>

      <WmTable :data="filteredSites" v-loading="loading" empty-title="该分类下还没有站点" empty-description="点击「创建站点」，按类型创建；HTML/PHP 为文件站，其余分类为反向代理到本机后端。">
        <WmColumn label="站点" min-width="150"><template #default="{ row }"><b>{{ row.name }}</b></template></WmColumn>
        <WmColumn label="类型" width="110">
          <template #default="{ row }"><WmStatusPill :label="typeLabel(row)" tone="neutral" :dot="false" outline /></template>
        </WmColumn>
        <WmColumn label="域名" min-width="210"><template #default="{ row }"><span class="wm-mono">{{ row.domains }}</span></template></WmColumn>
        <WmColumn label="SSL" width="90">
          <template #default="{ row }">
            <WmStatusPill v-if="row.ssl_cert" label="HTTPS" tone="success" :dot="false" />
            <span v-else class="wm-text-muted">—</span>
          </template>
        </WmColumn>
        <WmColumn label="启用" width="80">
          <template #default="{ row }"><a-switch :checked="row.enabled && row.conf_exists" @change="toggle(row)" /></template>
        </WmColumn>
        <WmColumn label="操作" width="290" fixed="right">
          <template #default="{ row }">
            <a-button size="small" @click="openSiteBackup(row)">备份</a-button>
            <a-button size="small" @click="viewLog(row, 'access')">访问日志</a-button>
            <a-button size="small" @click="viewLog(row, 'error')">错误日志</a-button>
            <a-button size="small" @click="openEdit(row)">编辑</a-button>
            <a-button size="small" data-tone="danger" @click="del(row)">删除</a-button>
          </template>
        </WmColumn>
      </WmTable>
    </WmSectionCard>

    <a-modal v-model:open="dlg" :title="form.id ? '编辑站点' : '创建站点'" width="680px">
      <a-form :label-col="{ style: { width: '110px' } }">
        <a-form-item label="站点标识"><a-input v-model:value="form.name" placeholder="小写字母数字点短横线，如 myblog" :disabled="!!form.id" /></a-form-item>
        <a-form-item label="站点类型">
          <a-radio-group v-model:value="form.type">
            <a-radio-button value="static">静态站</a-radio-button>
            <a-radio-button value="php">PHP 站</a-radio-button>
            <a-radio-button value="proxy">反向代理</a-radio-button>
          </a-radio-group>
        </a-form-item>
        <a-form-item label="域名"><a-input v-model:value="form.domains" placeholder="example.com www.example.com（空格分隔多个）" /></a-form-item>
        <a-form-item v-if="form.type !== 'proxy'" label="网站根目录"><a-input v-model:value="form.root" placeholder="/www/wwwroot/myblog" /></a-form-item>
        <a-form-item v-if="form.type === 'php'" label="PHP 版本">
          <a-select v-if="phpOptions.length" v-model:value="phpChoice" option-label-prop="label" style="width: 100%" allow-clear placeholder="选择面板已安装的 PHP 版本">
            <a-select-option v-for="p in phpOptions" :key="p.value" :value="p.value" :label="`PHP ${p.version}`">
              PHP {{ p.version }}<span class="wm-mono wm-text-muted" style="font-size: 12px; margin-left: 8px">{{ p.value }}</span>
            </a-select-option>
            <a-select-option :value="PHP_CUSTOM" label="手动输入 FPM 地址…">手动输入 FPM 地址…</a-select-option>
          </a-select>
          <a-input v-if="!phpOptions.length || phpChoice === PHP_CUSTOM" v-model:value="form.php_socket" placeholder="/run/php/php8.2-fpm.sock 或 127.0.0.1:9000" :class="{ 'wm-mt-2': phpOptions.length }" />
          <WmNotice v-if="!phpOptions.length" tone="warning" class="wm-mt-2">未检测到面板安装的 PHP：请先到「软件商店」安装 PHP 后刷新，或直接填写 FPM 地址</WmNotice>
        </a-form-item>
        <a-form-item v-if="form.type === 'proxy'" label="项目分类">
          <a-select v-model:value="form.sub_type" style="width: 100%">
            <a-select-option value="">无（纯反代）</a-select-option>
            <a-select-option value="java">Java 项目</a-select-option>
            <a-select-option value="node">Node 项目</a-select-option>
            <a-select-option value="go">Go 项目</a-select-option>
            <a-select-option value="python">Python 项目</a-select-option>
            <a-select-option value="dotnet">.Net 项目</a-select-option>
            <a-select-option value="other">其他项目</a-select-option>
          </a-select>
          <div class="hint">分类仅用于列表 Tab 区分展示；Java/Node/Go/Python/.Net 项目 = 反向代理到本机端口，请先在「进程管理」启动后端服务。</div>
        </a-form-item>
        <a-form-item v-if="form.type === 'proxy'" label="反代目标"><a-input v-model:value="form.upstream" placeholder="http://127.0.0.1:3000" /></a-form-item>
        <a-form-item v-if="form.type === 'proxy'" label="附加路径规则">
          <div v-for="(loc, i) in form.extra" :key="i" class="loc-row">
            <a-input v-model:value="loc.prefix" placeholder="/api" style="width: 160px" />
            <a-input v-model:value="loc.target" placeholder="http://127.0.0.1:8000" style="width: 300px" />
            <a-button size="small" shape="circle" data-tone="danger" @click="form.extra.splice(i, 1)"><span class="wm-icon"><Delete /></span></a-button>
          </div>
          <a-button size="small" @click="form.extra.push({ prefix: '', target: '' })">添加规则</a-button>
        </a-form-item>
        <a-form-item label="SSL 证书">
          <a-select v-model:value="form.ssl_cert" placeholder="选择已托管证书或留空" style="width: 100%" allow-clear>
            <a-select-option v-for="ct in certs" :key="ct.domain" :value="ct.domain">{{ `${ct.domain}（至 ${ct.not_after?.slice(0, 10)}）` }}</a-select-option>
          </a-select>
          <WmNotice v-if="form.ssl_cert" tone="neutral" class="wm-mt-2">将使用 <span class="wm-mono">{{ certBase }}</span> 路径下 fullchain.pem / privkey.pem</WmNotice>
        </a-form-item>
        <a-form-item v-if="form.ssl_cert" label="强制 HTTPS"><a-switch v-model:checked="form.force_https" /></a-form-item>
      </a-form>
      <template #footer>
        <a-button @click="dlg = false">取消</a-button>
        <a-button type="primary" :loading="saving" @click="save">保存（nginx -t 校验）</a-button>
      </template>
    </a-modal>

    <a-modal v-model:open="logDlg" :title="`日志 - ${logSite} (${logType})`" width="820px">
      <WmCodeBlock :content="logContent" :label="`${logSite} / ${logType}.log`" empty-text="（日志为空）" />
    </a-modal>

    <!-- 站点数据备份 -->
    <a-modal v-model:open="bakDlg" :title="'数据备份 - ' + (bakSite?.name || '')" width="760px" :footer="null">
      <WmToolbar>
        <a-button type="primary" size="small" :loading="bakCreating" @click="createSiteBackup">立即备份</a-button>
        <a-button size="small" @click="loadSiteBackups">刷新</a-button>
        <template #note>打包站点根目录与 Nginx 配置为 tar.gz，可下载导出、上传导入恢复。</template>
      </WmToolbar>
      <WmTable :data="siteBackups" size="small" v-loading="bakLoading" empty-title="暂无备份" empty-description="点击「立即备份」生成第一个备份包。">
        <WmColumn label="文件" min-width="300"><template #default="{ row }"><span class="wm-mono">{{ row.file }}</span></template></WmColumn>
        <WmColumn label="大小" width="110"><template #default="{ row }">{{ fmtSize(row.size) }}</template></WmColumn>
        <WmColumn label="时间" width="170"><template #default="{ row }">{{ row.time }}</template></WmColumn>
        <WmColumn label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <a-button size="small" @click="downloadSiteBackup(row)">下载</a-button>
            <a-button size="small" @click="restoreSiteBackup(row)">恢复</a-button>
            <a-button size="small" data-tone="danger" @click="deleteSiteBackup(row)">删除</a-button>
          </template>
        </WmColumn>
      </WmTable>
      <p class="hint" style="margin-top:8px">恢复前当前数据自动留存为 *.pre-restore-*.tar.gz 安全备份；也支持「上传恢复」导入外部 tar.gz。</p>
      <WmToolbar>
        <input type="file" accept=".tar.gz,.tgz" @change="e => uploadBakFile = e.target?.files?.[0] || null" />
        <a-button size="small" :disabled="!uploadBakFile" @click="uploadRestore">上传恢复</a-button>
      </WmToolbar>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from '../antd-compat'
import { Delete } from '../antd-compat'
import api from '../api'

// 类型显示：subtype 优先（Java/Node/...），否则按 type
const typeLabel = (row: any) => {
  const st = row.sub_type || ''
  if (st) return ({ java: 'Java', node: 'Node', go: 'Go', python: 'Python', dotnet: '.Net', other: '其他' } as any)[st] || st
  return ({ static: 'HTML', php: 'PHP', proxy: '反代' } as any)[row.type] || row.type
}

const nginx = ref<any>({})
const sites = ref<any[]>([])
const typeFilter = ref('all')

// 分类归一：proxy → proxy；static → html；php → php；proxy+sub_type → java/node/go/python/dotnet/other
function siteCategory(row: any): string {
  if (row.type === 'proxy') return 'proxy'
  if (row.type === 'php') return 'php'
  if (row.type === 'static') return 'html'
  const st = row.sub_type || ''
  if (['java', 'node', 'go', 'python', 'dotnet'].includes(st)) return st
  return 'other'
}

const filteredSites = computed(() => {
  if (typeFilter.value === 'all') return sites.value
  return sites.value.filter((r: any) => siteCategory(r) === typeFilter.value)
})

const certs = ref<any[]>([])
const loading = ref(false)
const dlg = ref(false)
const saving = ref(false)
const installing = ref(false)

const emptyForm = () => ({ id: 0, name: '', type: 'static', sub_type: '', domains: '', root: '', php_socket: '', upstream: '', extra: [] as any[], ssl_cert: '', force_https: false })
const form = reactive(emptyForm())
const certBase = ref('')

// PHP 版本下拉：读取面板（运行时模块）已安装的 PHP，选中即得到对应 FPM socket
const PHP_CUSTOM = '__custom__'
const phpRt = ref<any[]>([])
const phpChoice = ref('')

// 由 FPM systemd 单元名推导 socket：Debian/Ubuntu php8.2-fpm → /run/php/php8.2-fpm.sock；RHEL/Remi php-fpm → /run/php-fpm/www.sock
function phpSocketOf(inst: any) {
  if (inst.binary === 'php-fpm') return '/run/php-fpm/www.sock'
  if (inst.binary) return `/run/php/${inst.binary}.sock`
  return `/run/php/php${inst.version}-fpm.sock`
}

const phpOptions = computed(() => {
  const list = phpRt.value.map((inst) => ({ value: phpSocketOf(inst), version: String(inst.version || '') }))
  list.sort((a, b) => {
    const pa = a.version.split('.').map(Number)
    const pb = b.version.split('.').map(Number)
    for (let i = 0; i < 2; i++) {
      const d = (pb[i] || 0) - (pa[i] || 0)
      if (d) return d
    }
    return 0
  })
  return list
})

watch(phpChoice, (v) => {
  if (v !== PHP_CUSTOM) form.php_socket = v || ''
})

async function load() {
  loading.value = true
  try {
    nginx.value = await api.get('/sites/nginx/status')
    sites.value = (await api.get('/sites')) as any
    try {
      certs.value = (await api.get('/certs')) as any
    } catch {
      certs.value = []
    }
    try {
      const rts = (await api.get('/runtimes')) as any
      phpRt.value = rts.find((r: any) => r.key === 'php')?.installed || []
    } catch {
      phpRt.value = []
    }
  } finally {
    loading.value = false
  }
}

function openCreate() {
  Object.assign(form, emptyForm())
  form.extra = []
  phpChoice.value = phpOptions.value[0]?.value || ''
  form.php_socket = phpChoice.value
  dlg.value = true
}

function openEdit(row: any) {
  const s = row
  let extra: any[] = []
  try {
    extra = s.locations ? JSON.parse(s.locations) : []
  } catch {
    extra = []
  }
  Object.assign(form, emptyForm(), {
    id: s.id, name: s.name, type: s.type, sub_type: s.sub_type || '', domains: s.domains, root: s.root,
    php_socket: s.php_socket, upstream: s.upstream,
    ssl_cert: s.ssl_cert ? s.ssl_cert.replace(/.*certs-managed\//, '').replace(/\/fullchain\.pem$/, '') : '',
    force_https: s.force_https, extra
  })
  const known = phpOptions.value.find((p) => p.value === form.php_socket)
  phpChoice.value = known ? known.value : form.php_socket ? PHP_CUSTOM : ''
  dlg.value = true
}

function payload() {
  const locations = form.extra.filter((l) => l.prefix && l.target)
  const certPath = form.ssl_cert ? `/opt/hanyuxin/data/certs-managed/${form.ssl_cert}/fullchain.pem` : ''
  const keyPath = form.ssl_cert ? `/opt/hanyuxin/data/certs-managed/${form.ssl_cert}/privkey.pem` : ''
  return {
    name: form.name, type: form.type, sub_type: form.type === 'proxy' ? form.sub_type : '', domains: form.domains, root: form.root,
    php_socket: form.php_socket, upstream: form.upstream,
    locations: JSON.stringify(locations),
    ssl_cert: certPath, ssl_key: keyPath, force_https: form.force_https
  }
}

async function save() {
  saving.value = true
  try {
    if (form.id) await api.put(`/sites/${form.id}`, payload())
    else await api.post('/sites', payload())
    dlg.value = false
    ElMessage.success('已保存并重载 Nginx')
    load()
  } finally {
    saving.value = false
  }
}

async function toggle(row: any) {
  await api.post(`/sites/${row.id}/toggle`)
  load()
}

async function del(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除站点「${row.name}」？（仅移除 Nginx 配置，不删除网站文件）`, '删除站点', { type: 'warning' })
  } catch {
    return
  }
  await api.delete(`/sites/${row.id}`)
  load()
}

const logDlg = ref(false)
const logSite = ref('')
const logType = ref('access')
const logContent = ref('')

async function viewLog(row: any, type: string) {
  const d: any = await api.get(`/sites/${row.id}/log`, { params: { type, lines: 200 } })
  logSite.value = row.name
  logType.value = type
  logContent.value = d.content
  logDlg.value = true
}

async function reloadNginx() {
  await api.post('/sites/nginx/reload')
  ElMessage.success('已重载')
}

async function installNginx() {
  installing.value = true
  try {
    await api.post('/sites/nginx/install', {}, { timeout: 600000 })
    ElMessage.success('安装完成')
    load()
  } finally {
    installing.value = false
  }
}

// 选择证书时展示提示基础路径
watch(
  () => form.ssl_cert,
  (v) => {
    certBase.value = v ? `opt/hanyuxin/data/certs-managed/${v}` : ''
  }
)

onMounted(load)

// ---- 站点数据备份 ----
const bakDlg = ref(false)
const bakSite = ref<any>(null)
const siteBackups = ref<any[]>([])
const bakLoading = ref(false)
const bakCreating = ref(false)
const uploadBakFile = ref<File | null>(null)

function fmtSize(n: number) {
  if (n < 1024) return n + ' B'
  if (n < 1048576) return (n / 1024).toFixed(1) + ' KB'
  return (n / 1048576).toFixed(1) + ' MB'
}

function openSiteBackup(row: any) {
  bakSite.value = row
  bakDlg.value = true
  loadSiteBackups()
}

async function loadSiteBackups() {
  bakLoading.value = true
  try {
    siteBackups.value = (await api.get('/sites/backups')) as any
  } catch {
    siteBackups.value = []
  } finally {
    bakLoading.value = false
  }
}

async function createSiteBackup() {
  if (!bakSite.value?.id) return
  bakCreating.value = true
  try {
    const r: any = await api.post('/sites/' + bakSite.value.id + '/backup', {}, { timeout: 1800000 })
    ElMessage.success('备份完成：' + (r?.file || ''))
    loadSiteBackups()
  } finally {
    bakCreating.value = false
  }
}

function downloadSiteBackup(row: any) {
  window.open('/api/sites/backups/download?file=' + encodeURIComponent(row.file))
}

async function restoreSiteBackup(row: any) {
  try {
    await ElMessageBox.confirm('将用备份「' + row.file + '」覆盖站点数据并自动留存当前数据，继续？', '恢复确认', { type: 'warning' })
  } catch { return }
  const fd = new FormData()
  fd.append('id', String(bakSite.value.id))
  fd.append('file', row.file)
  const r: any = await api.post('/sites/backups/restore', fd, { timeout: 1800000, headers: { 'Content-Type': 'multipart/form-data' } })
  ElMessage.success(r?.message || '恢复完成')
  loadSiteBackups()
}

async function deleteSiteBackup(row: any) {
  try {
    await ElMessageBox.confirm('删除备份「' + row.file + '」？', '删除', { type: 'warning' })
  } catch { return }
  await api.delete('/sites/backups', { params: { file: row.file } })
  loadSiteBackups()
}

async function uploadRestore() {
  if (!uploadBakFile.value || !bakSite.value?.id) return
  try {
    await ElMessageBox.confirm('上传的备份将覆盖站点数据并自动留存当前数据，继续？', '上传恢复', { type: 'warning' })
  } catch { return }
  const fd = new FormData()
  fd.append('id', String(bakSite.value.id))
  fd.append('file', uploadBakFile.value)
  const r: any = await api.post('/sites/backups/restore', fd, { timeout: 1800000, headers: { 'Content-Type': 'multipart/form-data' } })
  ElMessage.success(r?.message || '恢复完成')
  uploadBakFile.value = null
  loadSiteBackups()
}
</script>

<style scoped>
.type-tabs {
  margin: 8px 0 4px;
}
.loc-row {
  display: flex;
  gap: 8px;
  margin-bottom: 6px;
}
.hint {
  color: var(--wm-text-2, #8492a6);
  font-size: 12px;
  line-height: 1.6;
  margin-top: 4px;
}
</style>
