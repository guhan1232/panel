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
        <WmNotice tone="info" title="修改入口需重启服务" class="wm-mt-4">面板端口与安全入口在 config.yaml 中配置，修改后需执行 systemctl restart digong。</WmNotice>
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
</script>
