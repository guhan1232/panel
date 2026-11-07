<template>
  <div>
    <el-card class="page-card">
      <div class="toolbar">
        <el-button type="primary" @click="issueDlg = true">Let's Encrypt 签发</el-button>
        <el-button @click="upDlg = true">上传证书</el-button>
        <el-button @click="load">刷新</el-button>
        <span class="hint">签发使用 HTTP-01 校验，需 80 端口可达本服务器；ACME 证书剩余 30 天内自动续期</span>
      </div>
      <el-table :data="certs" v-loading="loading">
        <el-table-column prop="domain" label="主域名" min-width="160" />
        <el-table-column label="包含域名" min-width="240"><template #default="{ row }">{{ (row.domains || []).join(', ') }}</template></el-table-column>
        <el-table-column label="到期时间" width="120"><template #default="{ row }">{{ row.not_after?.slice(0, 10) }}</template></el-table-column>
        <el-table-column label="剩余" width="100">
          <template #default="{ row }">
            <el-tag size="small" :type="row.days_left < 15 ? 'danger' : row.days_left < 30 ? 'warning' : 'success'">{{ row.days_left }} 天</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="来源" width="90">
          <template #default="{ row }"><el-tag size="small" :type="row.source === 'acme' ? 'primary' : 'info'">{{ row.source }}</el-tag></template>
        </el-table-column>
        <el-table-column label="操作" width="90">
          <template #default="{ row }">
            <el-button size="small" type="danger" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="issueDlg" title="Let's Encrypt 签发" width="560px">
      <el-form label-width="110px">
        <el-form-item label="域名列表"><el-input v-model="issue.domains" placeholder="example.com, www.example.com" /></el-form-item>
        <el-form-item label="邮箱"><el-input v-model="issue.email" placeholder="用于 Let's Encrypt 账户注册" /></el-form-item>
        <el-form-item label="测试环境"><el-switch v-model="issue.staging" /><span class="hint">staging 有速率限制宽松，验证流程可用后再切正式</span></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="issueDlg = false">取消</el-button>
        <el-button type="primary" :loading="issuing" @click="doIssue">开始签发（最长 3 分钟）</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="upDlg" title="上传证书" width="640px">
      <el-form label-width="90px">
        <el-form-item label="证书 PEM"><el-input v-model="up.cert" type="textarea" :rows="7" placeholder="-----BEGIN CERTIFICATE-----（fullchain）" /></el-form-item>
        <el-form-item label="私钥 PEM"><el-input v-model="up.key" type="textarea" :rows="5" placeholder="-----BEGIN PRIVATE KEY-----" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="upDlg = false">取消</el-button>
        <el-button type="primary" @click="doUpload">上传</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
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

<style scoped>
.hint { color: #999; font-size: 12px; }
</style>
