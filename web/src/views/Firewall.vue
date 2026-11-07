<template>
  <div>
    <el-card class="page-card">
      <div class="toolbar">
        <el-tag :type="fw.tool === 'none' ? 'danger' : 'success'" size="small">{{ toolLabel }}</el-tag>
        <el-switch v-if="fw.tool !== 'none'" v-model="fw.enabled" active-text="启用防火墙" @change="switchFw" />
        <el-button size="small" @click="load">刷新</el-button>
      </div>
      <el-alert v-if="fw.tool === 'none'" type="warning" :closable="false" :title="fw.message || '未检测到 ufw 或 firewalld'" />
      <template v-else>
        <div class="toolbar" style="margin-top: 12px">
          <el-input v-model="newPort" placeholder="如 8080/tcp" style="width: 200px" />
          <el-button type="primary" @click="addPort">放行端口</el-button>
        </div>
        <el-table :data="fw.ports" size="small">
          <el-table-column label="端口/协议" min-width="160"><template #default="{ row }">{{ row }}</template></el-table-column>
          <el-table-column label="操作" width="110">
            <template #default="{ row }">
              <el-button size="small" type="danger" @click="delPort(row)">移除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </template>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

const fw = ref<any>({ tool: 'none', enabled: false, ports: [], message: '' })
const newPort = ref('')

const toolLabel = computed(() => ({ ufw: 'ufw (Debian/Ubuntu)', firewalld: 'firewalld (RHEL)', none: '未检测到防火墙' } as any)[fw.value.tool] || fw.value.tool)

async function load() {
  fw.value = { ...fw.value, ...(await api.get('/firewall/status')) }
  if (fw.value.tool !== 'none') {
    const p: any = await api.get('/firewall/ports')
    fw.value.ports = p.ports
  }
}

async function addPort() {
  const v = newPort.value.trim()
  if (!/^\d+(-\d+)?\/(tcp|udp)$/.test(v)) {
    ElMessage.warning('格式应为 8080/tcp 或 53/udp')
    return
  }
  await api.post('/firewall/port', { port: v, action: 'add' })
  ElMessage.success('已放行')
  newPort.value = ''
  load()
}

async function delPort(p: string) {
  try {
    await ElMessageBox.confirm(`确认移除 ${p} 的放行规则？`, '确认', { type: 'warning' })
  } catch {
    return
  }
  await api.post('/firewall/port', { port: p, action: 'remove' })
  load()
}

async function switchFw() {
  await api.post('/firewall/switch', { enabled: fw.value.enabled })
  ElMessage.success(fw.value.enabled ? '防火墙已启用' : '防火墙已关闭')
}

onMounted(load)
</script>
