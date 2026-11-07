<template>
  <div>
    <el-card class="page-card">
      <div class="toolbar">
        <el-button type="primary" @click="openCreate">新建告警规则</el-button>
        <el-button @click="load">刷新</el-button>
        <span class="hint">每 30 秒评估一次；触发后按冷却时间去重，通过「通知与存储」中配置的渠道推送</span>
      </div>
      <el-table :data="rules" v-loading="loading">
        <el-table-column prop="name" label="规则" min-width="140" />
        <el-table-column label="条件" min-width="220">
          <template #default="{ row }">{{ metricLabel(row.metric) }} {{ row.op }} {{ row.threshold }}，持续 {{ row.duration_min }} 分钟</template>
        </el-table-column>
        <el-table-column label="冷却" width="80"><template #default="{ row }">{{ row.cooldown_min }}分</template></el-table-column>
        <el-table-column label="最近触发" width="165"><template #default="{ row }">{{ fmtTime(row.last_fired_at) }}</template></el-table-column>
        <el-table-column label="启用" width="80">
          <template #default="{ row }"><el-switch :model-value="row.enabled" @change="toggle(row)" /></template>
        </el-table-column>
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="del(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card header="触发历史">
      <el-table :data="events" size="small" v-loading="eLoading">
        <el-table-column prop="rule_name" label="规则" min-width="130" />
        <el-table-column label="指标" width="130"><template #default="{ row }">{{ metricLabel(row.metric) }}</template></el-table-column>
        <el-table-column prop="value" label="当时值" width="100" />
        <el-table-column prop="message" label="详情" min-width="320" />
        <el-table-column label="时间" width="165"><template #default="{ row }">{{ fmtTime(row.created_at) }}</template></el-table-column>
      </el-table>
      <el-pagination style="margin-top: 12px; justify-content: flex-end" layout="prev, pager, next, total" :total="eTotal" :page-size="eSize" v-model:current-page="ePage" @current-change="loadEvents" />
    </el-card>

    <el-dialog v-model="dlg" :title="form.id ? '编辑规则' : '新建规则'" width="540px">
      <el-form label-width="110px">
        <el-form-item label="规则名称"><el-input v-model="form.name" placeholder="如 CPU 过载" /></el-form-item>
        <el-form-item label="监控指标">
          <el-select v-model="form.metric" style="width: 100%">
            <el-option v-for="(v, k) in metricMap" :key="k" :label="v" :value="k" />
          </el-select>
        </el-form-item>
        <el-form-item label="触发条件">
          <el-select v-model="form.op" style="width: 90px">
            <el-option label="大于 >" value=">" />
            <el-option label="小于 <" value="<" />
          </el-select>
          <el-input-number v-model="form.threshold" :precision="1" :step="5" style="margin-left: 8px; width: 140px" />
        </el-form-item>
        <el-form-item label="持续(分钟)"><el-input-number v-model="form.duration_min" :min="0" /><span class="hint">0 = 立即触发</span></el-form-item>
        <el-form-item label="冷却(分钟)"><el-input-number v-model="form.cooldown_min" :min="5" :step="5" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dlg = false">取消</el-button>
        <el-button type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'
import { fmtTime } from '../utils'

const metricMap: Record<string, string> = { cpu: 'CPU 使用率(%)', mem: '内存使用率(%)', disk: '磁盘使用率(%)', load1: '1分钟负载' }
const metricLabel = (k: string) => metricMap[k] || k

const rules = ref<any[]>([])
const events = ref<any[]>([])
const loading = ref(false)
const eLoading = ref(false)
const ePage = ref(1)
const eSize = 10
const eTotal = ref(0)
const dlg = ref(false)

const emptyForm = () => ({ id: 0, name: '', metric: 'cpu', op: '>', threshold: 90, duration_min: 3, cooldown_min: 30 })
const form = reactive(emptyForm())

async function load() {
  loading.value = true
  try {
    rules.value = (await api.get('/alerts/rules')) as any
  } finally {
    loading.value = false
  }
  loadEvents()
}

async function loadEvents() {
  eLoading.value = true
  try {
    const d: any = await api.get('/alerts/events', { params: { page: ePage.value, size: eSize } })
    events.value = d.items
    eTotal.value = d.total
  } finally {
    eLoading.value = false
  }
}

function openCreate() {
  Object.assign(form, emptyForm())
  dlg.value = true
}

function openEdit(row: any) {
  Object.assign(form, emptyForm(), row)
  dlg.value = true
}

async function save() {
  if (form.id) await api.put(`/alerts/rules/${form.id}`, { ...form })
  else await api.post('/alerts/rules', { ...form })
  dlg.value = false
  ElMessage.success('已保存')
  load()
}

async function toggle(row: any) {
  await api.post(`/alerts/rules/${row.id}/toggle`)
  load()
}

async function del(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除规则「${row.name}」？`, '删除', { type: 'warning' })
  } catch {
    return
  }
  await api.delete(`/alerts/rules/${row.id}`)
  load()
}

onMounted(load)
</script>

<style scoped>
.hint { color: #999; font-size: 12px; margin-left: 8px; }
</style>
