<template>
  <div>
    <el-card class="page-card">
      <div class="toolbar">
        <el-tag :type="nginx.installed ? 'success' : 'danger'" size="small">
          Nginx {{ nginx.installed ? nginx.version || '已安装' : '未安装' }}
        </el-tag>
        <el-button v-if="!nginx.installed" size="small" type="warning" :loading="installing" @click="installNginx">安装 Nginx</el-button>
        <el-button size="small" @click="reloadNginx">重载 Nginx</el-button>
        <el-button type="primary" size="small" @click="openCreate">创建站点</el-button>
        <el-button size="small" @click="load">刷新</el-button>
      </div>
      <el-table :data="sites" v-loading="loading">
        <el-table-column label="站点" min-width="140">
          <template #default="{ row }"><b>{{ row.site.name }}</b></template>
        </el-table-column>
        <el-table-column label="类型" width="90">
          <template #default="{ row }"><el-tag size="small">{{ typeLabel(row.site.type) }}</el-tag></template>
        </el-table-column>
        <el-table-column label="域名" min-width="200"><template #default="{ row }">{{ row.site.domains }}</template></el-table-column>
        <el-table-column label="SSL" width="70">
          <template #default="{ row }"><el-tag v-if="row.site.ssl_cert" type="success" size="small">HTTPS</el-tag><span v-else>-</span></template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-switch :model-value="row.site.enabled && row.conf_exists" @change="toggle(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="250" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="viewLog(row, 'access')">访问日志</el-button>
            <el-button size="small" @click="viewLog(row, 'error')">错误日志</el-button>
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="dlg" :title="form.id ? '编辑站点' : '创建站点'" width="680px">
      <el-form label-width="110px">
        <el-form-item label="站点标识"><el-input v-model="form.name" placeholder="小写字母数字点短横线，如 myblog" :disabled="!!form.id" /></el-form-item>
        <el-form-item label="站点类型">
          <el-radio-group v-model="form.type">
            <el-radio-button value="static">静态站</el-radio-button>
            <el-radio-button value="php">PHP 站</el-radio-button>
            <el-radio-button value="proxy">反向代理</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="域名"><el-input v-model="form.domains" placeholder="example.com www.example.com（空格分隔多个）" /></el-form-item>
        <el-form-item v-if="form.type !== 'proxy'" label="网站根目录"><el-input v-model="form.root" placeholder="/www/wwwroot/myblog" /></el-form-item>
        <el-form-item v-if="form.type === 'php'" label="PHP-FPM"><el-input v-model="form.php_socket" placeholder="/run/php/php8.2-fpm.sock 或 127.0.0.1:9000" /></el-form-item>
        <el-form-item v-if="form.type === 'proxy'" label="反代目标"><el-input v-model="form.upstream" placeholder="http://127.0.0.1:3000" /></el-form-item>
        <el-form-item v-if="form.type === 'proxy'" label="附加路径规则">
          <div v-for="(loc, i) in form.extra" :key="i" class="loc-row">
            <el-input v-model="loc.prefix" placeholder="/api" style="width: 160px" />
            <el-input v-model="loc.target" placeholder="http://127.0.0.1:8000" style="width: 300px" />
            <el-button size="small" circle type="danger" @click="form.extra.splice(i, 1)"><el-icon><Delete /></el-icon></el-button>
          </div>
          <el-button size="small" @click="form.extra.push({ prefix: '', target: '' })">添加规则</el-button>
        </el-form-item>
        <el-form-item label="SSL 证书">
          <el-select v-model="form.ssl_cert" placeholder="选择已托管证书或留空" style="width: 100%" clearable>
            <el-option v-for="ct in certs" :key="ct.domain" :label="`${ct.domain}（至 ${ct.not_after?.slice(0, 10)}）`" :value="ct.domain" />
          </el-select>
          <div class="hint" v-if="form.ssl_cert">将使用 /{{ certBase }} 路径下 fullchain.pem / privkey.pem</div>
        </el-form-item>
        <el-form-item v-if="form.ssl_cert" label="强制 HTTPS"><el-switch v-model="form.force_https" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dlg = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存（nginx -t 校验）</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="logDlg" :title="`日志 - ${logSite} (${logType})`" width="820px">
      <pre class="pre-output">{{ logContent || '（空）' }}</pre>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Delete } from '@element-plus/icons-vue'
import api from '../api'

const typeMap: Record<string, string> = { static: '静态', php: 'PHP', proxy: '反代' }
const typeLabel = (t: string) => typeMap[t] || t

const nginx = ref<any>({})
const sites = ref<any[]>([])
const certs = ref<any[]>([])
const loading = ref(false)
const dlg = ref(false)
const saving = ref(false)
const installing = ref(false)

const emptyForm = () => ({ id: 0, name: '', type: 'static', domains: '', root: '', php_socket: '', upstream: '', extra: [] as any[], ssl_cert: '', force_https: false })
const form = reactive(emptyForm())
const certBase = ref('')

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
  } finally {
    loading.value = false
  }
}

function openCreate() {
  Object.assign(form, emptyForm())
  form.extra = []
  dlg.value = true
}

function openEdit(row: any) {
  const s = row.site
  let extra: any[] = []
  try {
    extra = s.locations ? JSON.parse(s.locations) : []
  } catch {
    extra = []
  }
  Object.assign(form, emptyForm(), {
    id: s.id, name: s.name, type: s.type, domains: s.domains, root: s.root,
    php_socket: s.php_socket, upstream: s.upstream,
    ssl_cert: s.ssl_cert ? s.ssl_cert.replace(/.*certs-managed\//, '').replace(/\/fullchain\.pem$/, '') : '',
    force_https: s.force_https, extra
  })
  dlg.value = true
}

function payload() {
  const locations = form.extra.filter((l) => l.prefix && l.target)
  const certPath = form.ssl_cert ? `/opt/webmblx/data/certs-managed/${form.ssl_cert}/fullchain.pem` : ''
  const keyPath = form.ssl_cert ? `/opt/webmblx/data/certs-managed/${form.ssl_cert}/privkey.pem` : ''
  return {
    name: form.name, type: form.type, domains: form.domains, root: form.root,
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
  await api.post(`/sites/${row.site.id}/toggle`)
  load()
}

async function del(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除站点「${row.site.name}」？（仅移除 Nginx 配置，不删除网站文件）`, '删除站点', { type: 'warning' })
  } catch {
    return
  }
  await api.delete(`/sites/${row.site.id}`)
  load()
}

const logDlg = ref(false)
const logSite = ref('')
const logType = ref('access')
const logContent = ref('')

async function viewLog(row: any, type: string) {
  const d: any = await api.get(`/sites/${row.site.id}/log`, { params: { type, lines: 200 } })
  logSite.value = row.site.name
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
    certBase.value = v ? `opt/webmblx/data/certs-managed/${v}` : ''
  }
)

onMounted(load)
</script>

<style scoped>
.loc-row { display: flex; gap: 8px; margin-bottom: 6px; }
.hint { color: #999; font-size: 12px; margin-top: 4px; }
</style>
