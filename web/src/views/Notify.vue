<template>
  <div class="wm-page">
    <div class="wm-grid-2 wm-grid-2--equal">
      <div class="wm-stack">
        <WmSectionCard title="邮件通知 (SMTP)" description="计划任务失败、证书到期等告警通过邮件送达" marker>
          <a-form :label-col="{ style: { width: '100px' } }">
            <a-form-item label="启用"><a-switch v-model:checked="email.enabled" /></a-form-item>
            <a-form-item label="SMTP 服务器"><a-input v-model:value="email.host" placeholder="smtp.example.com" /></a-form-item>
            <a-form-item label="端口"><a-input-number v-model:value="email.port" :min="1" controls-position="right" /></a-form-item>
            <a-form-item label="SSL(465)"><a-switch v-model:checked="email.ssl" /></a-form-item>
            <a-form-item label="账号"><a-input v-model:value="email.user" /></a-form-item>
            <a-form-item label="密码/授权码"><a-input-password v-model:value="email.pass"  placeholder="QQ邮箱请填授权码" /></a-form-item>
            <a-form-item label="发件人"><a-input v-model:value="email.from" placeholder="留空则使用账号" /></a-form-item>
            <a-form-item label="收件人"><a-input v-model:value="email.toStr" placeholder="多个用英文逗号分隔" /></a-form-item>
          </a-form>
          <template #footer>
            <a-button @click="test('email')">发送测试邮件</a-button>
            <a-button type="primary" @click="saveEmail">保存</a-button>
          </template>
        </WmSectionCard>

        <WmSectionCard title="S3 兼容对象存储" description="计划任务备份可同时归档到远端桶" marker>
          <a-form :label-col="{ style: { width: '100px' } }">
            <a-form-item label="启用"><a-switch v-model:checked="s3.enabled" /></a-form-item>
            <a-form-item label="Endpoint"><a-input v-model:value="s3.endpoint" placeholder="cos.ap-guangzhou.myqcloud.com" /></a-form-item>
            <a-form-item label="AccessKey"><a-input v-model:value="s3.access_key" /></a-form-item>
            <a-form-item label="SecretKey"><a-input-password v-model:value="s3.secret_key" /></a-form-item>
            <a-form-item label="桶名"><a-input v-model:value="s3.bucket" /></a-form-item>
            <a-form-item label="HTTPS"><a-switch v-model:checked="s3.use_ssl" /></a-form-item>
            <a-form-item label="前缀"><a-input v-model:value="s3.prefix" placeholder="backups/" /></a-form-item>
          </a-form>
          <template #footer>
            <a-button @click="testS3">测试连通性</a-button>
            <a-button type="primary" @click="saveS3">保存</a-button>
          </template>
        </WmSectionCard>
      </div>

      <div class="wm-stack">
        <WmSectionCard title="IM Webhook 通知" description="钉钉 / 企业微信 / 飞书 / 通用 JSON" marker>
          <a-form :label-col="{ style: { width: '100px' } }">
            <a-form-item label="启用"><a-switch v-model:checked="hook.enabled" /></a-form-item>
            <a-form-item label="类型">
              <a-select v-model:value="hook.type" style="width: 100%">
                <a-select-option value="dingtalk">钉钉</a-select-option>
                <a-select-option value="wecom">企业微信</a-select-option>
                <a-select-option value="feishu">飞书</a-select-option>
                <a-select-option value="generic">通用 JSON</a-select-option>
              </a-select>
            </a-form-item>
            <a-form-item label="Webhook URL"><a-textarea v-model:value="hook.url"  :rows="2" /></a-form-item>
          </a-form>
          <template #footer>
            <a-button @click="test('webhook')">发送测试消息</a-button>
            <a-button type="primary" @click="saveHook">保存</a-button>
          </template>
        </WmSectionCard>

      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive } from 'vue'
import { ElMessage } from '../antd-compat'
import api from '../api'

const email = reactive<any>({ enabled: false, host: '', port: 465, ssl: true, user: '', pass: '', from: '', toStr: '' })
const hook = reactive<any>({ enabled: false, type: 'dingtalk', url: '' })
const s3 = reactive<any>({ enabled: false, endpoint: '', access_key: '', secret_key: '', bucket: '', use_ssl: true, prefix: 'backups/' })

async function load() {
  const cfg: any = await api.get('/notify/config')
  Object.assign(email, cfg.email, { toStr: (cfg.email.to || []).join(',') })
  Object.assign(hook, cfg.webhook)
  const s: any = await api.get('/s3/config')
  Object.assign(s3, s)
}

async function saveEmail() {
  const { toStr, ...rest } = email
  await api.post('/notify/email', { ...rest, to: toStr.split(/[,;\s]+/).filter(Boolean) })
  ElMessage.success('已保存')
}

async function saveHook() {
  await api.post('/notify/webhook', { ...hook })
  ElMessage.success('已保存')
}

async function saveS3() {
  await api.post('/s3/config', { ...s3 })
  ElMessage.success('已保存')
}

async function test(channel: string) {
  await saveEmail()
  await api.post('/notify/test', { channel })
  ElMessage.success('发送成功，请查收')
}

async function testS3() {
  await saveS3()
  await api.post('/s3/test')
  ElMessage.success('桶连接正常')
}

onMounted(load)
</script>
