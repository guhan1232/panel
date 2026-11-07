<template>
  <div class="wm-page">
    <WmSectionCard title="用户列表" description="三角色 RBAC：admin 全部权限 · operator 可操作全部功能 · viewer 只读且禁用终端" flush>
      <WmToolbar>
        <el-button type="primary" @click="openCreate">新建用户</el-button>
        <el-button @click="load">刷新</el-button>
        <template #note>不能修改自身角色、禁用或删除自身，以保护最后一个可用管理员。</template>
      </WmToolbar>
      <WmTable :data="users" v-loading="loading" empty-title="暂无用户">
        <el-table-column prop="id" label="ID" width="70"><template #default="{ row }"><span class="wm-mono">{{ row.id }}</span></template></el-table-column>
        <el-table-column prop="username" label="用户名" min-width="160"><template #default="{ row }"><b>{{ row.username }}</b></template></el-table-column>
        <el-table-column label="角色" width="130">
          <template #default="{ row }"><WmStatusPill :label="roleLabel(row.role)" :tone="row.role === 'admin' ? 'danger' : row.role === 'operator' ? 'info' : 'neutral'" :dot="false" /></template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }"><WmStatusPill :label="row.disabled ? '已禁用' : '正常'" :tone="row.disabled ? 'danger' : 'success'" /></template>
        </el-table-column>
        <el-table-column label="创建时间" width="175"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.created_at) }}</span></template></el-table-column>
        <el-table-column label="操作" width="230" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" :disabled="row.username === auth.username" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
      </WmTable>
    </WmSectionCard>

    <el-dialog v-model="dlg" :title="form.id ? `编辑 - ${form.username}` : '新建用户'" width="480px">
      <el-form label-width="100px">
        <el-form-item label="用户名">
          <el-input v-model="form.username" :disabled="!!form.id" placeholder="2-32 位字母/数字/下划线" />
        </el-form-item>
        <el-form-item :label="form.id ? '重置密码' : '密码'">
          <el-input v-model="form.password" type="password" show-password :placeholder="form.id ? '留空则不修改' : '至少 8 位'" />
        </el-form-item>
        <el-form-item label="角色">
          <el-select v-model="form.role" style="width: 100%" :disabled="form.username === auth.username">
            <el-option label="admin — 全部权限" value="admin" />
            <el-option label="operator — 可操作全部功能" value="operator" />
            <el-option label="viewer — 只读" value="viewer" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.id" label="禁用">
          <el-switch v-model="form.disabled" :disabled="form.username === auth.username" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dlg = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
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
