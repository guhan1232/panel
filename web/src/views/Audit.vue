<template>
  <div class="wm-page">
    <WmSectionCard title="审计记录" description="写操作留痕：操作人、中文操作说明、脱敏请求体、来源与状态；终端命令逐条记录" flush>
      <WmToolbar>
        <a-segmented v-model:value="tab" :options="[{ value: 'ops', label: '操作日志' }, { value: 'tty', label: '终端命令' }]" size="small" />
        <template v-if="tab === 'ops'">
          <a-select v-model:value="q.method" placeholder="请求方法" allow-clear size="small" style="width: 120px">
            <a-select-option v-for="m in ['POST', 'PUT', 'DELETE']" :key="m" :value="m">{{ m }}</a-select-option>
          </a-select>
          <a-input v-model:value="q.q" placeholder="路径关键字" size="small" style="width: 200px" allow-clear />
          <a-input v-model:value="q.username" placeholder="操作人" size="small" style="width: 130px" allow-clear />
        </template>
        <template v-else>
          <a-input v-model:value="tq.user" placeholder="操作人" size="small" style="width: 130px" allow-clear />
          <a-input v-model:value="tq.q" placeholder="命令关键字" size="small" style="width: 200px" allow-clear />
        </template>
        <template #end>
          <a-button v-if="tab === 'ops'" type="primary" size="small" @click="load(1)">查询</a-button>
          <a-button v-else type="primary" size="small" @click="loadT(1)">查询</a-button>
        </template>
      </WmToolbar>

      <!-- 操作日志 -->
      <div v-if="tab === 'ops'" class="wm-section-block wm-section-block--table">
        <WmTable :data="items" v-loading="loading" size="small" empty-title="没有匹配的记录" empty-description="调整筛选条件后重新查询。">
          <WmColumn prop="id" label="ID" width="70"><template #default="{ row }"><span class="wm-mono">{{ row.id }}</span></template></WmColumn>
          <WmColumn prop="username" label="操作人" width="130" />
          <WmColumn label="角色" width="90">
            <template #default="{ row }">
              <WmStatusPill v-if="row.role" :label="row.role" :tone="row.role === 'admin' ? 'info' : row.role === 'operator' ? 'warning' : 'neutral'" :dot="false" mono />
              <span v-else class="wm-text-muted">—</span>
            </template>
          </WmColumn>
          <WmColumn label="操作" min-width="150"><template #default="{ row }"><span style="white-space: nowrap">{{ row.action && row.action !== row.method + ' ' + row.path ? row.action : '—' }}</span></template></WmColumn>
          <WmColumn prop="method" label="方法" width="80">
            <template #default="{ row }"><WmStatusPill :label="row.method" :tone="row.method === 'DELETE' ? 'danger' : 'info'" :dot="false" mono /></template>
          </WmColumn>
          <WmColumn prop="path" label="路径" min-width="200"><template #default="{ row }"><span class="wm-mono">{{ row.path }}</span></template></WmColumn>
          <WmColumn label="请求体" width="90">
            <template #default="{ row }">
              <a-button v-if="row.body" type="link" size="small" @click="viewBody(row)">查看</a-button>
              <span v-else class="wm-text-muted">—</span>
            </template>
          </WmColumn>
          <WmColumn prop="ip" label="来源 IP" width="130"><template #default="{ row }"><span class="wm-mono">{{ row.ip }}</span></template></WmColumn>
          <WmColumn prop="status" label="状态" width="75"><template #default="{ row }"><span class="wm-mono">{{ row.status }}</span></template></WmColumn>
          <WmColumn label="耗时" width="85"><template #default="{ row }"><span class="wm-mono">{{ row.cost_ms }}ms</span></template></WmColumn>
          <WmColumn label="时间" width="165"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.created_at) }}</span></template></WmColumn>
        </WmTable>
        <div class="wm-pagination-pad">
          <a-pagination :hide-on-single-page="true" :total="total" :page-size="size" v-model:current="page" @change="load()" />
        </div>
      </div>

      <!-- 终端命令 -->
      <div v-else class="wm-section-block wm-section-block--table">
        <WmTable :data="titems" v-loading="tloading" size="small" empty-title="没有终端命令记录" empty-description="Web 终端中按回车执行的命令行会记录在此。">
          <WmColumn prop="id" label="ID" width="70"><template #default="{ row }"><span class="wm-mono">{{ row.id }}</span></template></WmColumn>
          <WmColumn prop="username" label="操作人" width="130" />
          <WmColumn label="命令行" min-width="420"><template #default="{ row }"><span class="wm-mono">{{ row.line }}</span></template></WmColumn>
          <WmColumn label="时间" width="165"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.created_at) }}</span></template></WmColumn>
        </WmTable>
        <div class="wm-pagination-pad">
          <a-pagination :hide-on-single-page="true" :total="ttotal" :page-size="size" v-model:current="tpage" @change="loadT()" />
        </div>
      </div>
    </WmSectionCard>

    <a-modal v-model:open="bodyDlg" :title="`请求体 - ${bodyRow.method} ${bodyRow.path}`" width="680px">
      <pre class="pre-output">{{ prettyBody }}</pre>
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import api from '../api'
import { fmtTime } from '../utils'

const tab = ref('ops')
const items = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const size = 20
const loading = ref(false)
const q = reactive({ method: '', q: '', username: '' })

const titems = ref<any[]>([])
const ttotal = ref(0)
const tpage = ref(1)
const tloading = ref(false)
const tq = reactive({ user: '', q: '' })

const bodyDlg = ref(false)
const bodyRow = ref<any>({})
const prettyBody = computed(() => {
  try {
    return JSON.stringify(JSON.parse(bodyRow.value.body || ''), null, 2)
  } catch {
    return bodyRow.value.body || ''
  }
})

function viewBody(row: any) {
  bodyRow.value = row
  bodyDlg.value = true
}

async function load(p?: number) {
  if (p) page.value = p
  loading.value = true
  try {
    const d: any = await api.get('/audit/list', { params: { page: page.value, size, ...q } })
    items.value = d.items
    total.value = d.total
  } finally {
    loading.value = false
  }
}

async function loadT(p?: number) {
  if (p) tpage.value = p
  tloading.value = true
  try {
    const d: any = await api.get('/terminal-logs', { params: { page: tpage.value, size, ...tq } })
    titems.value = d.items
    ttotal.value = d.total
  } finally {
    tloading.value = false
  }
}

onMounted(() => load())
</script>
