<template>
  <div class="wm-page">
    <WmSectionCard title="用户列表" description="三角色 RBAC：admin 全部权限 · operator 可操作全部功能 · viewer 只读且禁用终端" flush>
      <WmToolbar>
        <a-button type="primary" size="small" @click="openCreate">新建用户</a-button>
        <a-button size="small" @click="load">刷新</a-button>
        <template #note>不能修改自身角色、禁用或删除自身，以保护最后一个可用管理员。</template>
      </WmToolbar>
      <WmTable :data="users" v-loading="loading" empty-title="暂无用户">
        <WmColumn prop="id" label="ID" width="70"><template #default="{ row }"><span class="wm-mono">{{ row.id }}</span></template></WmColumn>
        <WmColumn prop="username" label="用户名" min-width="160"><template #default="{ row }"><b>{{ row.username }}</b></template></WmColumn>
        <WmColumn label="角色" width="130">
          <template #default="{ row }"><WmStatusPill :label="roleLabel(row.role)" :tone="row.role === 'admin' ? 'info' : row.role === 'operator' ? 'warning' : 'neutral'" :dot="false" /></template>
        </WmColumn>
        <WmColumn label="状态" width="110">
          <template #default="{ row }"><WmStatusPill :label="row.disabled ? '已禁用' : '正常'" :tone="row.disabled ? 'danger' : 'success'" /></template>
        </WmColumn>
        <WmColumn label="创建时间" width="175"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.created_at) }}</span></template></WmColumn>
        <WmColumn label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <a-button type="link" size="small" @click="openEdit(row)">编辑</a-button>
            <a-button type="link" size="small" danger :disabled="row.username === auth.username" @click="del(row)">删除</a-button>
          </template>
        </WmColumn>
      </WmTable>
    </WmSectionCard>

    <a-modal v-model:open="dlg" :title="form.id ? `编辑 - ${form.username}` : '新建用户'" width="480px">
      <a-form layout="vertical">
        <a-form-item label="用户名">
          <a-input v-model:value="form.username" :disabled="!!form.id" placeholder="2-32 位字母/数字/下划线" />
        </a-form-item>
        <a-form-item :label="form.id ? '重置密码' : '密码'">
          <a-input-password v-model:value="form.password"  :placeholder="form.id ? '留空则不修改' : '至少 8 位'" />
        </a-form-item>
        <a-form-item label="角色">
          <a-select v-model:value="form.role" style="width: 100%" :disabled="form.username === auth.username">
            <a-select-option value="admin">admin — 全部权限</a-select-option>
            <a-select-option value="operator">operator — 可操作全部功能</a-select-option>
            <a-select-option value="viewer">viewer — 只读</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item v-if="form.id" label="禁用">
          <a-switch v-model:checked="form.disabled" :disabled="form.username === auth.username" />
        </a-form-item>
      </a-form>
      <template #footer>
        <a-button @click="dlg = false">取消</a-button>
        <a-button type="primary" :loading="saving" @click="save">保存</a-button>
      </template>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from '../antd-compat'
import api from '../api'
import { fmtTime } from '../utils'
import { useAuth } from '../stores/auth'

const auth = useAuth()
const users = ref<any[]>([])
const loading = ref(false)
const dlg = ref(false)
const saving = ref(false)

const roleMap: Record<string, string> = { admin: '管理员', operator: '操作员', viewer: '只读' }
const roleLabel = (r: string) => roleMap[r] || r
const roleType = (r: string) => ({ admin: 'danger', operator: 'primary', viewer: 'info' } as any)[r] || 'info'

const emptyForm = () => ({ id: 0, username: '', password: '', role: 'operator', disabled: false })
const form = reactive(emptyForm())

async function load() {
  loading.value = true
  try {
    users.value = (await api.get('/users')) as any
  } finally {
    loading.value = false
  }
}

function openCreate() {
  Object.assign(form, emptyForm())
  dlg.value = true
}

function openEdit(row: any) {
  Object.assign(form, emptyForm(), {
    id: row.id, username: row.username, role: row.role, disabled: !!row.disabled, password: ''
  })
  dlg.value = true
}

async function save() {
  saving.value = true
  try {
    if (form.id) {
      const payload: any = { role: form.role, disabled: form.disabled }
      if (form.password) payload.password = form.password
      await api.put(`/users/${form.id}`, payload)
    } else {
      await api.post('/users', { username: form.username, password: form.password, role: form.role })
    }
    dlg.value = false
    ElMessage.success('已保存')
    load()
  } finally {
    saving.value = false
  }
}

async function del(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除用户「${row.username}」？其会话将立即失效`, '删除用户', { type: 'warning' })
  } catch {
    return
  }
  await api.delete(`/users/${row.id}`)
  ElMessage.success('已删除')
  load()
}

onMounted(load)
</script>
