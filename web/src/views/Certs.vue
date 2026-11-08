<template>
  <div class="wm-page">
    <WmSectionCard title="证书列表" description="ACME 证书剩余 30 天内自动续期，到期前推送告警" flush>
      <WmToolbar>
        <a-button type="primary" @click="issueDlg = true">Let's Encrypt 签发</a-button>
        <a-button @click="upDlg = true">上传证书</a-button>
        <a-button @click="load">刷新</a-button>
        <template #note>签发使用 HTTP-01 校验，需 80 端口可达本服务器。</template>
      </WmToolbar>
      <WmTable :data="certs" v-loading="loading" empty-title="还没有托管证书" empty-description="上传已有证书，或通过 Let's Encrypt 自动签发。">
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
        <WmColumn label="操作" width="100">
          <template #default="{ row }"><a-button size="small" data-tone="danger" @click="del(row)">删除</a-button></template>
        </WmColumn>
      </WmTable>
    </WmSectionCard>

    <a-modal v-model:open="issueDlg" title="Let's Encrypt 签发" width="560px">
      <a-form :label-col="{ style: { width: '110px' } }">
        <a-form-item label="域名列表"><a-input v-model:value="issue.domains" placeholder="example.com, www.example.com" /></a-form-item>
        <a-form-item label="邮箱"><a-input v-model:value="issue.email" placeholder="用于 Let's Encrypt 账户注册" /></a-form-item>
        <a-form-item label="测试环境"><a-switch v-model:checked="issue.staging" /><span class="hint">staging 速率限制宽松，验证流程可用后再切正式</span></a-form-item>
      </a-form>
      <template #footer>
        <a-button @click="issueDlg = false">取消</a-button>
        <a-button type="primary" :loading="issuing" @click="doIssue">开始签发（最长 3 分钟）</a-button>
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
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from '../antd-compat'
import api from '../api'

const certs = ref<any[]>([])
const loading = ref(false)
const issueDlg = ref(false)
const issuing = ref(false)
const upDlg = ref(false)

const issue = reactive({ domains: '', email: '', staging: true })
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
