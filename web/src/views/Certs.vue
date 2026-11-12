<template>
  <div class="wm-page">
    <WmSectionCard title="证书列表" description="ACME 证书剩余 30 天内自动续期，到期前推送告警" flush>
      <template #actions>
        <span class="wm-text-muted cert-stats">{{ certs.length }} 张证书 · {{ expiringCount }} 张即将到期</span>
      </template>
      <WmToolbar>
        <template #end>
          <a-button size="small" @click="upDlg = true">上传证书</a-button>
          <a-button type="primary" size="small" @click="openIssue">申请证书</a-button>
          <a-button size="small" @click="load">刷新</a-button>
        </template>
        <template #note>签发使用 HTTP-01 校验，需 80 端口可达本服务器。</template>
      </WmToolbar>
      <WmTable :data="certs" v-loading="loading" empty-title="还没有托管证书" empty-description="上传已有证书，或通过 Let's Encrypt 自动签发。">
        <template #empty>
          <div class="cert-empty">
            <div class="cert-empty__icon"><span class="wm-icon"><Stamp /></span></div>
            <p class="cert-empty__title">还没有托管证书</p>
            <p class="cert-empty__desc">上传已有证书，或通过 Let's Encrypt 自动签发；ACME 证书剩余 30 天内自动续期。</p>
            <a-button type="primary" @click="openIssue">申请证书</a-button>
          </div>
        </template>
        <WmColumn prop="domain" label="主域名" min-width="170"><template #default="{ row }"><b class="wm-mono">{{ row.domain }}</b></template></WmColumn>
        <WmColumn label="包含域名" min-width="250"><template #default="{ row }"><span class="wm-mono">{{ (row.domains || []).join(', ') }}</span></template></WmColumn>
        <WmColumn label="到期时间" width="130"><template #default="{ row }"><span class="wm-mono">{{ row.not_after?.slice(0, 10) }}</span></template></WmColumn>
        <WmColumn label="剩余" width="110">
          <template #default="{ row }">
            <WmStatusPill :label="`${row.days_left} 天`" :tone="row.days_left < 15 ? 'danger' : row.days_left < 30 ? 'warning' : 'success'" />
          </template>
        </WmColumn>
        <WmColumn label="来源" width="100">
          <template #default="{ row }"><WmStatusPill :label="row.source" :tone="row.source === 'acme' ? 'info' : 'neutral'" :dot="false" mono /></template>
        </WmColumn>
        <WmColumn label="操作" width="130" fixed="right">
          <template #default="{ row }">
            <a-button type="link" size="small" @click="openDeploy(row)">部署</a-button>
            <a-button type="link" size="small" danger @click="del(row)">删除</a-button>
          </template>
        </WmColumn>
      </WmTable>
    </WmSectionCard>

    <a-drawer v-model:open="issueDlg" title="Let's Encrypt 签发" :width="620">
      <a-form layout="vertical">
        <div class="form-sec">
          <div class="form-sec__title">基本信息</div>
          <a-form-item label="域名列表"><a-input v-model:value="issue.domains" placeholder="example.com, www.example.com" /></a-form-item>
          <a-form-item label="邮箱" style="margin-bottom: 0"><a-input v-model:value="issue.email" placeholder="用于 Let's Encrypt 账户注册" /></a-form-item>
        </div>
        <div class="form-sec">
          <div class="form-sec__title">校验方式</div>
          <a-form-item label="挑战类型">
            <a-radio-group v-model:value="issue.challenge" option-type="button" button-style="solid">
              <a-radio-button value="http01">HTTP-01</a-radio-button>
              <a-radio-button value="dns01">DNS-01</a-radio-button>
            </a-radio-group>
            <div class="hint">泛域名（*.example.com）必须使用 DNS-01</div>
          </a-form-item>
          <template v-if="issue.challenge === 'dns01'">
            <a-form-item label="DNS 服务商">
              <a-select v-model:value="issue.dns_provider" style="width: 100%" show-search>
                <a-select-option v-for="sp in dnsProviders" :key="sp.id" :value="sp.id">{{ sp.name }}</a-select-option>
              </a-select>
            </a-form-item>
            <a-form-item v-for="f in curDnsFields" :key="f.key" :label="f.required ? f.label : (f.label.includes('可选') ? f.label : f.label + '（可选）')">
              <a-textarea v-if="f.textarea" v-model:value="issue.dns_fields[f.key]" :rows="4" class="wm-mono" :placeholder="f.placeholder" />
              <a-input-password v-else-if="f.secret" v-model:value="issue.dns_fields[f.key]" :placeholder="f.placeholder" />
              <a-input v-else v-model:value="issue.dns_fields[f.key]" :placeholder="f.placeholder" />
            </a-form-item>
          </template>
          <a-form-item label="测试环境" style="margin-bottom: 0">
            <div><a-switch v-model:checked="issue.staging" /></div>
            <div class="hint">staging 速率限制宽松，验证流程可用后再切正式</div>
          </a-form-item>
        </div>
      </a-form>
      <template #footer>
        <a-button @click="issueDlg = false">取消</a-button>
        <a-button type="primary" :loading="issuing" @click="doIssue">开始签发（最长 3 分钟）</a-button>
      </template>
    </a-drawer>

    <a-modal v-model:open="deployDlg" :title="`部署 ${deployRow?.domain || ''} 到网站`" width="520px">
      <a-form :label-col="{ style: { width: '110px' } }">
        <a-form-item label="目标网站">
          <a-select v-model:value="deploySite" style="width: 100%">
            <a-select-option v-for="st in sites" :key="st.id" :value="st.id">{{ st.name }}（{{ st.domains }}）</a-select-option>
          </a-select>
          <span class="hint">部署后站点将启用 HTTPS 并强制跳转；证书域名需与站点域名匹配。</span>
        </a-form-item>
      </a-form>
      <template #footer>
        <a-button @click="deployDlg = false">取消</a-button>
        <a-button type="primary" :loading="deploying" @click="doDeploy">部署并启用 HTTPS</a-button>
      </template>
    </a-modal>

    <a-modal v-model:open="upDlg" title="上传证书" width="640px">
      <WmNotice tone="neutral" class="wm-mb-4">上传后将校验证书与私钥是否配对；fullchain 与私钥需同时提供。</WmNotice>
      <a-form :label-col="{ style: { width: '90px' } }">
        <a-form-item label="证书 PEM"><a-textarea v-model:value="up.cert"  :rows="7" placeholder="-----BEGIN CERTIFICATE-----（fullchain）" /></a-form-item>
        <a-form-item label="私钥 PEM"><a-textarea v-model:value="up.key"  :rows="5" placeholder="-----BEGIN PRIVATE KEY-----" /></a-form-item>
      </a-form>
      <template #footer>
        <a-button @click="upDlg = false">取消</a-button>
        <a-button type="primary" @click="doUpload">上传</a-button>
      </template>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, watch, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, Stamp } from '../antd-compat'
import api from '../api'

const certs = ref<any[]>([])
const loading = ref(false)
const issueDlg = ref(false)
const issuing = ref(false)
const upDlg = ref(false)
const expiringCount = computed(() => certs.value.filter((c: any) => (c.days_left ?? 999) < 30).length)

const issue = reactive({ domains: '', email: '', staging: true, challenge: 'http01', dns_provider: 'alidns', dns_api_key: '', dns_api_secret: '', dns_fields: {} as Record<string, string> })
const dnsProviders = ref<any[]>([])
const curDnsFields = computed(() => dnsProviders.value.find((sp: any) => sp.id === issue.dns_provider)?.fields || [])

async function loadDnsProviders() {
  try {
    dnsProviders.value = (await api.get('/certs/dns-providers')) as any
  } catch {
    dnsProviders.value = []
  }
}
watch(() => issue.dns_provider, () => { issue.dns_fields = {} })

function openIssue() {
  if (!dnsProviders.value.length) loadDnsProviders()
  issueDlg.value = true
}

const deployDlg = ref(false)
const deployRow = ref<any>(null)
const deploySite = ref<number | null>(null)
const sites = ref<any[]>([])
const deploying = ref(false)

async function openDeploy(row: any) {
  deployRow.value = row
  deploySite.value = null
  try {
    sites.value = (await api.get('/sites')) as any
  } catch {
    sites.value = []
  }
  deployDlg.value = true
}

async function doDeploy() {
  if (!deploySite.value) {
    ElMessage.warning('请选择目标网站')
    return
  }
  deploying.value = true
  try {
    const d: any = await api.post('/sites/apply-cert', { site_id: deploySite.value, cert_domain: deployRow.value.domain })
    ElMessage.success('已部署到站点 ' + (d.site || '') + ' 并启用 HTTPS')
    deployDlg.value = false
  } finally {
    deploying.value = false
  }
}
const up = reactive({ cert: '', key: '' })

async function load() {
  loading.value = true
  try {
    certs.value = (await api.get('/certs')) as any
  } finally {
    loading.value = false
  }
}

async function doIssue() {
  if (!issue.domains || !issue.email) {
    ElMessage.warning('请填写域名与邮箱')
    return
  }
  issuing.value = true
  try {
    await api.post('/certs/issue', { domains: issue.domains.split(/[,;\s]+/).filter(Boolean), email: issue.email, staging: issue.staging }, { timeout: 200000 })
    ElMessage.success('签发成功')
    issueDlg.value = false
    load()
  } finally {
    issuing.value = false
  }
}

async function doUpload() {
  if (!up.cert || !up.key) {
    ElMessage.warning('请填写证书与私钥')
    return
  }
  await api.post('/certs/upload', { cert_pem: up.cert, key_pem: up.key })
  ElMessage.success('已上传')
  upDlg.value = false
  up.cert = up.key = ''
  load()
}

async function del(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除证书「${row.domain}」？引用它的站点需手动更换证书`, '删除证书', { type: 'warning' })
  } catch {
    return
  }
  await api.delete(`/certs/${row.domain}`)
  load()
}

onMounted(load)
</script>

<style scoped>
.cert-stats { font-size: 12px; }

/* 空态 */
.cert-empty {
  display: flex; flex-direction: column; align-items: center; text-align: center;
  padding: 48px 20px 52px;
}
.cert-empty__icon {
  width: 44px; height: 44px; border-radius: 50%;
  background: var(--wm-surface-3); color: var(--wm-ink-3);
  display: grid; place-items: center; font-size: 20px; margin-bottom: 12px;
}
.cert-empty__title { font-size: 14px; font-weight: 500; color: var(--wm-ink); }
.cert-empty__desc { font-size: 12px; color: var(--wm-ink-3); margin: 4px 0 14px; max-width: 46ch; line-height: 1.7; }

/* 抽屉表单分区 */
.form-sec { padding-bottom: 20px; margin-bottom: 20px; border-bottom: 1px solid var(--wm-line); }
.form-sec:last-child { border-bottom: 0; margin-bottom: 0; padding-bottom: 0; }
.form-sec__title { font-size: 12px; font-weight: 600; color: var(--wm-ink-3); letter-spacing: 0.06em; margin-bottom: 14px; }
</style>
