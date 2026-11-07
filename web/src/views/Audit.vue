<template>
  <div class="wm-page">
    <WmSectionCard title="审计记录" description="全部写操作留痕：操作人、角色、来源 IP、状态与耗时" flush>
      <WmToolbar>
        <el-select v-model="q.method" placeholder="请求方法" clearable style="width: 130px">
          <el-option v-for="m in ['POST', 'PUT', 'DELETE']" :key="m" :label="m" :value="m" />
        </el-select>
        <el-input v-model="q.q" placeholder="路径关键字" style="width: 240px" clearable />
        <el-input v-model="q.username" placeholder="操作人" style="width: 140px" clearable />
        <el-button type="primary" @click="load(1)">查询</el-button>
        <template #note>共 {{ total }} 条记录</template>
      </WmToolbar>
      <WmTable :data="items" v-loading="loading" size="small" empty-title="没有匹配的记录" empty-description="调整筛选条件后重新查询。">
        <el-table-column prop="id" label="ID" width="70"><template #default="{ row }"><span class="wm-mono">{{ row.id }}</span></template></el-table-column>
        <el-table-column prop="username" label="操作人" width="110" />
        <el-table-column label="角色" width="100">
          <template #default="{ row }">
            <WmStatusPill v-if="row.role" :label="row.role" :tone="row.role === 'admin' ? 'danger' : row.role === 'operator' ? 'info' : 'neutral'" :dot="false" mono />
            <span v-else class="wm-text-muted">—</span>
          </template>
        </el-table-column>
        <el-table-column prop="method" label="方法" width="90">
          <template #default="{ row }"><WmStatusPill :label="row.method" :tone="row.method === 'DELETE' ? 'danger' : 'info'" :dot="false" mono /></template>
        </el-table-column>
        <el-table-column prop="path" label="路径" min-width="250"><template #default="{ row }"><span class="wm-mono">{{ row.path }}</span></template></el-table-column>
        <el-table-column prop="ip" label="来源 IP" width="140"><template #default="{ row }"><span class="wm-mono">{{ row.ip }}</span></template></el-table-column>
        <el-table-column prop="status" label="状态" width="80"><template #default="{ row }"><span class="wm-mono">{{ row.status }}</span></template></el-table-column>
        <el-table-column label="耗时" width="90"><template #default="{ row }"><span class="wm-mono">{{ row.cost_ms }}ms</span></template></el-table-column>
        <el-table-column label="时间" width="170"><template #default="{ row }"><span class="wm-mono">{{ fmtTime(row.created_at) }}</span></template></el-table-column>
      </WmTable>
      <div class="wm-pagination-pad">
        <el-pagination layout="prev, pager, next, total" :total="total" :page-size="size" v-model:current-page="page" @current-change="load()" />
      </div>
    </WmSectionCard>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import api from '../api'
import { fmtTime } from '../utils'

const items = ref<any[]>([])
const total = ref(0)
const page = ref(1)
const size = 20
const loading = ref(false)
const q = reactive({ method: '', q: '', username: '' })

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

onMounted(() => load())
</script>
