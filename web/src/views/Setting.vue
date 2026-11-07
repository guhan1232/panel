<template>
  <el-row :gutter="16">
    <el-col :span="12">
      <el-card header="面板信息">
        <el-descriptions :column="1" border>
          <el-descriptions-item label="版本">{{ info.version }}</el-descriptions-item>
          <el-descriptions-item label="监听地址">{{ info.listen }}</el-descriptions-item>
          <el-descriptions-item label="HTTPS">
            <el-tag size="small" :type="info.tls ? 'success' : 'warning'">{{ info.tls ? '已启用' : '未启用' }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="数据目录">{{ info.data_dir }}</el-descriptions-item>
          <el-descriptions-item label="运行环境">{{ info.go_version }} / {{ info.os }}-{{ info.arch }}</el-descriptions-item>
        </el-descriptions>
        <el-alert style="margin-top: 12px" type="info" :closable="false"
          title="提示" description="面板端口与安全入口在 config.yaml 中配置，修改后需重启面板服务（systemctl restart webmblx）。" />
      </el-card>
    </el-col>
    <el-col :span="12">
      <el-card header="修改管理员密码">
        <el-form label-width="100px" style="max-width: 420px">
          <el-form-item label="原密码"><el-input v-model="form.old" type="password" show-password /></el-form-item>
          <el-form-item label="新密码"><el-input v-model="form.n1" type="password" show-password placeholder="至少 8 位" /></el-form-item>
          <el-form-item label="确认密码"><el-input v-model="form.n2" type="password" show-password /></el-form-item>
          <el-button type="primary" :loading="saving" @click="submit">修改密码</el-button>
        </el-form>
      </el-card>
    </el-col>
  </el-row>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
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
