<template>
  <div class="wm-page">
    <div class="wm-grid-2 wm-grid-2--equal">
      <WmSectionCard title="面板信息" description="当前运行实例的基本信息" marker dense>
        <a-descriptions :column="1" bordered>
          <a-descriptions-item label="版本">{{ info.version }}</a-descriptions-item>
          <a-descriptions-item label="监听地址">{{ info.listen }}</a-descriptions-item>
          <a-descriptions-item label="HTTPS">
            <WmStatusPill :label="info.tls ? '已启用' : '未启用'" :tone="info.tls ? 'success' : 'warning'" :dot="false" />
          </a-descriptions-item>
          <a-descriptions-item label="数据目录">{{ info.data_dir }}</a-descriptions-item>
          <a-descriptions-item label="运行环境">{{ info.go_version }} / {{ info.os }}-{{ info.arch }}</a-descriptions-item>
        </a-descriptions>
        <WmNotice tone="info" title="修改入口需重启服务" class="wm-mt-4">面板端口与安全入口在 config.yaml 中配置，修改后需执行 systemctl restart hanyuxin。</WmNotice>
      </WmSectionCard>


    <WmSectionCard title="两因素验证（TOTP）" description="启用后登录需输入验证器 App 的 6 位动态码">
      <template #actions>
        <a-button size="small" type="primary" @click="setup2fa">开始绑定</a-button>
        <a-button size="small" data-tone="danger" @click="disable2fa">解除绑定</a-button>
      </template>
      <div v-if="faSecret">
        <p class="wm-text-muted">将以下密钥录入验证器（Google Authenticator / 微信小程序「腾讯 Token」等），然后输入动态码确认：</p>
        <p class="wm-mono"><b>{{ faSecret }}</b></p>
        <p class="hint wm-mono">{{ faURL }}</p>
        <a-input v-model:value="faCode" placeholder="6 位动态码" style="width: 220px" />
        <a-button type="primary" size="small" :loading="faSaving" @click="confirm2fa">确认启用</a-button>
      </div>
      <p v-else class="wm-text-muted">尚未绑定或已解除。</p>
    </WmSectionCard>    <WmSectionCard title="面板数据备份" description="打包 SQLite 数据库快照、config.yaml 与托管证书，下载留档">
      <template #actions>
        <a-button size="small" type="primary" @click="window && window.open('/api/setting/panel-backup')">下载备份包</a-button>
      </template>
      <p class="wm-text-muted">备份包含 panel.db（一致性好快照）、config.yaml（含安全入口）与 certs-managed 目录；恢复时停服解压覆盖对应文件即可。</p>
    </WmSectionCard>

      <WmSectionCard title="修改管理员密码" description="新密码至少 8 位，修改后当前会话保持有效" marker dense>
        <a-form :label-col="{ style: { width: '100px' } }" style="max-width: 440px">
          <a-form-item label="原密码"><a-input-password v-model:value="form.old" /></a-form-item>
          <a-form-item label="新密码"><a-input-password v-model:value="form.n1"  placeholder="至少 8 位" /></a-form-item>
          <a-form-item label="确认密码"><a-input-password v-model:value="form.n2" /></a-form-item>
          <a-button type="primary" :loading="saving" @click="submit">修改密码</a-button>
        </a-form>
      </WmSectionCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from '../antd-compat'
import api from '../api'

const info = ref<any>({})
const form = reactive({ old: '', n1: '', n2: '' })
const saving = ref(false)

async function load() {
  info.value = (await api.get('/setting/info')) as any
}

async function submit() {
  if (!form.old || !form.n1) {
    ElMessage.warning('请填写完整')
    return
  }
  if (form.n1 !== form.n2) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }
  if (form.n1.length < 8) {
    ElMessage.warning('新密码至少 8 位')
    return
  }
  saving.value = true
  try {
    await api.post('/setting/password', { old: form.old, new: form.n1 })
    ElMessage.success('密码已修改')
    form.old = form.n1 = form.n2 = ''
  } finally {
    saving.value = false
  }
}

onMounted(load)

const faSecret = ref('')
const faURL = ref('')
const faCode = ref('')
const faSaving = ref(false)

async function setup2fa() {
  const d: any = await api.post('/setting/2fa/setup')
  faSecret.value = d.secret
  faURL.value = d.url
  faCode.value = ''
}

async function confirm2fa() {
  faSaving.value = true
  try {
    await api.post('/setting/2fa/enable', { secret: faSecret.value, code: faCode.value })
    ElMessage.success('两因素验证已启用，下次登录需输入动态码')
    faSecret.value = ''
  } finally {
    faSaving.value = false
  }
}

async function disable2fa() {
  try {
    await ElMessageBox.confirm('确认解除两因素验证？需要输入登录密码', '解除绑定', { type: 'warning' })
  } catch { return }
  try {
    const r: any = await ElMessageBox.prompt('输入登录密码确认解除', '密码确认', { inputType: 'password' })
    await api.post('/setting/2fa/disable', { password: r.value })
    ElMessage.success('已解除')
  } catch { /* 取消或密码错误 */ }
}
</script>
