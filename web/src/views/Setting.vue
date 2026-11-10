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




    <WmSectionCard title="在线会话" description="当前账号在其他设备的登录会话，可踢出">
      <template #actions>
        <a-button size="small" @click="loadSessions">刷新</a-button>
      </template>
      <WmTable :data="sessions" size="small" empty-title="没有在线会话" empty-description="">
        <WmColumn label="IP" width="150"><template #default="{ row }"><span class="wm-mono">{{ row.ip }}</span></template></WmColumn>
        <WmColumn label="设备" min-width="260"><template #default="{ row }"><span class="wm-text-muted wm-truncate">{{ row.ua }}</span></template></WmColumn>
        <WmColumn label="过期" width="170"><template #default="{ row }"><span class="wm-mono">{{ row.expires }}</span></template></WmColumn>
        <WmColumn label="操作" width="110" fixed="right">
          <template #default="{ row }">
            <a-button v-if="!row.current" size="small" data-tone="danger" @click="revoke(row)">踢出</a-button>
            <WmStatusPill v-else label="本设备" tone="info" :dot="false" />
          </template>
        </WmColumn>
      </WmTable>
    </WmSectionCard>    <WmSectionCard title="面板在线升级" description="从升级清单检查新版本，下载校验后原子替换并自动重启">
      <template #actions>
        <a-button size="small" @click="checkUpgrade">检查更新</a-button>
        <a-button v-if="upInfo.has_update" size="small" type="primary" :loading="upApplying" @click="applyUpgrade">升级到 {{ upInfo.remote }}</a-button>
      </template>
      <div v-if="upInfo.current">
        <p>当前版本 <b class="wm-mono">{{ upInfo.current }}</b>
          <template v-if="upInfo.remote"> · 清单版本 <b class="wm-mono">{{ upInfo.remote }}</b>
          <WmStatusPill v-if="upInfo.has_update" label="有更新" tone="warning" />
          <WmStatusPill v-else-if="upInfo.reachable" label="已是最新" tone="success" />
          </template></p>
        <p v-if="upInfo.notes" class="wm-text-muted">{{ upInfo.notes }}</p>
        <p v-if="upInfo.err" class="wm-text-muted">清单不可达：{{ upInfo.err }}（可在配置 upgrade_manifest_url 更换升级源）</p>
      </div>
      <WmNotice tone="warning" class="wm-mt-4">升级会自动备份当前二进制为 *.bak 并重启服务，重启期间面板短暂不可访问。</WmNotice>
    </WmSectionCard>    <WmSectionCard title="两因素验证（TOTP）" description="启用后登录需输入验证器 App 的 6 位动态码">
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

      <WmSectionCard title="账号信息" description="修改登录用户名；初始随机用户名建议尽快改为易记名称">
      <template #actions>
        <a-button size="small" type="primary" :loading="savingName" @click="saveUsername">保存用户名</a-button>
      </template>
      <a-form layout="inline">
        <a-form-item label="当前用户名"><span class="wm-mono"><b>{{ auth.username }}</b></span></a-form-item>
        <a-form-item label="新用户名"><a-input v-model:value="newUsername" placeholder="3-64 位，字母/数字/_/-" style="width: 220px" /></a-form-item>
      </a-form>
      <WmNotice tone="neutral" class="wm-mt-4">修改后需用新用户名重新登录；SSH 首次输出的初始凭据文件将在你首次修改密码后自动删除。</WmNotice>
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

onMounted(loadSessions)
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

const upInfo = ref<any>({})
const upApplying = ref(false)

async function checkUpgrade() {
  upInfo.value = (await api.get('/setting/upgrade/check')) as any
}

async function applyUpgrade() {
  try {
    await ElMessageBox.confirm('升级会替换面板二进制并自动重启（约数秒不可访问），继续？', "在线升级", { type: 'warning' })
  } catch { return }
  upApplying.value = true
  try {
    await api.post('/setting/upgrade/apply', { binary_url: upInfo.value.binary_url, sha256: upInfo.value.sha256 || '' }, { timeout: 900000 })
    ElMessage.success('二进制已替换，面板正在重启，稍后刷新页面并核对版本')
  } finally {
    upApplying.value = false
  }
}

const auth = useAuth()
const sessions = ref<any[]>([])

async function loadSessions() {
  try {
    sessions.value = (await api.get('/auth/sessions')) as any
  } catch {
    sessions.value = []
  }
}

async function revoke(row: any) {
  try {
    await ElMessageBox.confirm('确认踢出该会话？', '踢出', { type: 'warning' })
  } catch { return }
  await api.delete('/auth/sessions/' + row.id)
  ElMessage.success('已踢出')
  loadSessions()
}

const newUsername = ref('')
const savingName = ref(false)

async function saveUsername() {
  const name = newUsername.value.trim()
  if (name.length < 3) {
    ElMessage.warning('用户名至少 3 位')
    return
  }
  savingName.value = true
  try {
    await api.post('/setting/profile', { username: name })
    ElMessage.success('用户名已改为 ' + name + '，下次登录使用新用户名')
    auth.username = name
    newUsername.value = ''
  } finally {
    savingName.value = false
  }
}
</script>
