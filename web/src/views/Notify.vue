<template>
  <div class="wm-page">
    <div class="wm-grid-2 wm-grid-2--equal">
      <div class="wm-stack">
        <WmSectionCard title="邮件通知 (SMTP)" description="计划任务失败、证书到期等告警通过邮件送达" marker>
          <el-form label-width="100px">
            <el-form-item label="启用"><el-switch v-model="email.enabled" /></el-form-item>
            <el-form-item label="SMTP 服务器"><el-input v-model="email.host" placeholder="smtp.example.com" /></el-form-item>
            <el-form-item label="端口"><el-input-number v-model="email.port" :min="1" controls-position="right" /></el-form-item>
            <el-form-item label="SSL(465)"><el-switch v-model="email.ssl" /></el-form-item>
            <el-form-item label="账号"><el-input v-model="email.user" /></el-form-item>
            <el-form-item label="密码/授权码"><el-input v-model="email.pass" type="password" show-password placeholder="QQ邮箱请填授权码" /></el-form-item>
            <el-form-item label="发件人"><el-input v-model="email.from" placeholder="留空则使用账号" /></el-form-item>
            <el-form-item label="收件人"><el-input v-model="email.toStr" placeholder="多个用英文逗号分隔" /></el-form-item>
          </el-form>
          <template #footer>
            <el-button @click="test('email')">发送测试邮件</el-button>
            <el-button type="primary" @click="saveEmail">保存</el-button>
          </template>
        </WmSectionCard>

        <WmSectionCard title="S3 兼容对象存储" description="计划任务备份可同时归档到远端桶" marker>
          <el-form label-width="100px">
            <el-form-item label="启用"><el-switch v-model="s3.enabled" /></el-form-item>
            <el-form-item label="Endpoint"><el-input v-model="s3.endpoint" placeholder="cos.ap-guangzhou.myqcloud.com" /></el-form-item>
            <el-form-item label="AccessKey"><el-input v-model="s3.access_key" /></el-form-item>
            <el-form-item label="SecretKey"><el-input v-model="s3.secret_key" type="password" show-password /></el-form-item>
            <el-form-item label="桶名"><el-input v-model="s3.bucket" /></el-form-item>
            <el-form-item label="HTTPS"><el-switch v-model="s3.use_ssl" /></el-form-item>
            <el-form-item label="前缀"><el-input v-model="s3.prefix" placeholder="backups/" /></el-form-item>
          </el-form>
          <template #footer>
            <el-button @click="testS3">测试连通性</el-button>
            <el-button type="primary" @click="saveS3">保存</el-button>
          </template>
        </WmSectionCard>
      </div>

      <div class="wm-stack">
        <WmSectionCard title="IM Webhook 通知" description="钉钉 / 企业微信 / 飞书 / 通用 JSON" marker>
          <el-form label-width="100px">
            <el-form-item label="启用"><el-switch v-model="hook.enabled" /></el-form-item>
            <el-form-item label="类型">
              <el-select v-model="hook.type" style="width: 100%">
                <el-option label="钉钉" value="dingtalk" />
                <el-option label="企业微信" value="wecom" />
                <el-option label="飞书" value="feishu" />
                <el-option label="通用 JSON" value="generic" />
              </el-select>
            </el-form-item>
            <el-form-item label="Webhook URL"><el-input v-model="hook.url" type="textarea" :rows="2" /></el-form-item>
          </el-form>
          <template #footer>
            <el-button @click="test('webhook')">发送测试消息</el-button>
            <el-button type="primary" @click="saveHook">保存</el-button>
          </template>
        </WmSectionCard>

      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive } from 'vue'
import { ElMessage } from 'element-plus'
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
