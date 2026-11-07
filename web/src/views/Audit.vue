<template>
  <el-card>
    <div class="toolbar">
      <el-select v-model="q.method" placeholder="请求方法" clearable style="width: 110px">
        <el-option v-for="m in ['POST', 'PUT', 'DELETE']" :key="m" :label="m" :value="m" />
      </el-select>
      <el-input v-model="q.q" placeholder="路径关键字" style="width: 240px" clearable />
      <el-input v-model="q.username" placeholder="操作人" style="width: 130px" clearable />
      <el-button type="primary" @click="load(1)">查询</el-button>
    </div>
    <el-table :data="items" v-loading="loading" size="small">
      <el-table-column prop="id" label="ID" width="70" />
      <el-table-column prop="username" label="操作人" width="100" />
      <el-table-column prop="method" label="方法" width="80">
        <template #default="{ row }"><el-tag size="small" :type="row.method === 'DELETE' ? 'danger' : 'primary'">{{ row.method }}</el-tag></template>
      </el-table-column>
      <el-table-column prop="path" label="路径" min-width="240" />
      <el-table-column prop="ip" label="来源 IP" width="130" />
      <el-table-column prop="status" label="状态" width="70" />
      <el-table-column label="耗时" width="80"><template #default="{ row }">{{ row.cost_ms }}ms</template></el-table-column>
      <el-table-column label="时间" width="165"><template #default="{ row }">{{ fmtTime(row.created_at) }}</template></el-table-column>
    </el-table>
    <el-pagination style="margin-top: 12px; justify-content: flex-end" layout="prev, pager, next, total" :total="total" :page-size="size" v-model:current-page="page" @current-change="load()" />
  </el-card>
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
