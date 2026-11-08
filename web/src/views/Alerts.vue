<template>
  <div class="wm-page">
    <WmSectionCard title="告警规则" description="阈值规则每 30 秒评估一次，触发后按冷却时间去重" flush>
      <WmToolbar>
        <a-button type="primary" @click="openCreate">新建告警规则</a-button>
        <a-button @click="load">刷新</a-button>
        <template #note>触发后经「通知与存储」中配置的渠道推送。</template>
      </WmToolbar>
      <WmTable :data="rules" v-loading="loading" empty-title="还没有告警规则" empty-description="设置 CPU、内存、磁盘或负载阈值，异常时通过通知渠道推送。">
        <WmColumn prop="name" label="规则" min-width="150"><template #default="{ row }"><b>{{ row.name }}</b></template></WmColumn>
        <WmColumn label="条件" min-width="230">
          <template #default="{ row }">{{ metricLabel(row.metric) }} {{ row.op }} {{ row.threshold }}，持续 {{ row.duration_min }} 分钟</template>
        </WmColumn>
        <WmColumn label="冷却" width="90"><template #default="{ row }"><span class="wm-mono">{{ row.cooldown_min }} 分</span></template></WmColumn>
        <WmColumn label="最近触发" width="170"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.last_fired_at) }}</span></template></WmColumn>
        <WmColumn label="启用" width="80">
          <template #default="{ row }"><a-switch :checked="row.enabled" @change="toggle(row)" /></template>
        </WmColumn>
        <WmColumn label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <a-button size="small" @click="openEdit(row)">编辑</a-button>
            <a-button size="small" data-tone="danger" @click="del(row)">删除</a-button>
          </template>
        </WmColumn>
      </WmTable>
    </WmSectionCard>

    <WmSectionCard title="触发历史" description="最近告警事件，按时间倒序" flush>
      <WmTable :data="events" size="small" v-loading="eLoading" empty-title="暂无触发记录">
        <WmColumn prop="rule_name" label="规则" min-width="140" />
        <WmColumn label="指标" width="140"><template #default="{ row }">{{ metricLabel(row.metric) }}</template></WmColumn>
        <WmColumn prop="value" label="当时值" width="110"><template #default="{ row }"><span class="wm-mono">{{ row.value }}</span></template></WmColumn>
        <WmColumn prop="message" label="详情" min-width="320" />
        <WmColumn label="时间" width="170"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.created_at) }}</span></template></WmColumn>
      </WmTable>
      <div class="wm-pagination-pad">
        <a-pagination :total="eTotal" :page-size="eSize" v-model:current="ePage" @change="loadEvents" />
      </div>
    </WmSectionCard>

    <a-modal v-model:open="dlg" :title="form.id ? '编辑规则' : '新建规则'" width="540px">
      <a-form :label-col="{ style: { width: '110px' } }">
        <a-form-item label="规则名称"><a-input v-model:value="form.name" placeholder="如 CPU 过载" /></a-form-item>
        <a-form-item label="监控指标">
          <a-select v-model:value="form.metric" style="width: 100%">
            <a-select-option v-for="(v, k) in metricMap" :key="k" :value="k">{{ v }}</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="触发条件">
          <a-select v-model:value="form.op" style="width: 90px">
            <a-select-option value=">">大于 ></a-select-option>
            <a-select-option value="<">小于 <</a-select-option>
          </a-select>
          <a-input-number v-model:value="form.threshold" :precision="1" :step="5" style="margin-left: 8px; width: 140px" />
        </a-form-item>
        <a-form-item label="持续(分钟)"><a-input-number v-model:value="form.duration_min" :min="0" /><span class="hint">0 = 立即触发</span></a-form-item>
        <a-form-item label="冷却(分钟)"><a-input-number v-model:value="form.cooldown_min" :min="5" :step="5" /></a-form-item>
      </a-form>
      <template #footer>
        <a-button @click="dlg = false">取消</a-button>
        <a-button type="primary" @click="save">保存</a-button>
      </template>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from '../antd-compat'
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
