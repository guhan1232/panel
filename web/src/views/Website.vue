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

      <WmTable :data="sites" v-loading="loading" empty-title="还没有站点" empty-description="创建静态站、PHP 站或反向代理，域名与证书可随时绑定。">
        <WmColumn label="站点" min-width="150"><template #default="{ row }"><b>{{ row.site.name }}</b></template></WmColumn>
        <WmColumn label="类型" width="100">
          <template #default="{ row }"><WmStatusPill :label="typeLabel(row.site.type)" tone="neutral" :dot="false" outline /></template>
        </WmColumn>
        <WmColumn label="域名" min-width="210"><template #default="{ row }"><span class="wm-mono">{{ row.site.domains }}</span></template></WmColumn>
        <WmColumn label="SSL" width="90">
          <template #default="{ row }">
            <WmStatusPill v-if="row.site.ssl_cert" label="HTTPS" tone="success" :dot="false" />
            <span v-else class="wm-text-muted">—</span>
          </template>
        </WmColumn>
        <WmColumn label="启用" width="80">
          <template #default="{ row }"><a-switch :checked="row.site.enabled && row.conf_exists" @change="toggle(row)" /></template>
        </WmColumn>
        <WmColumn label="操作" width="250" fixed="right">
          <template #default="{ row }">
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
        <a-form-item v-if="form.type === 'php'" label="PHP-FPM"><a-input v-model:value="form.php_socket" placeholder="/run/php/php8.2-fpm.sock 或 127.0.0.1:9000" /></a-form-item>
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
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from '../antd-compat'
import { Delete } from '../antd-compat'
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
