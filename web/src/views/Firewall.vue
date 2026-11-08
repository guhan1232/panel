<template>
  <div class="wm-page">
    <WmSectionCard title="防火墙状态" description="统一抽象 ufw 与 firewalld，操作即时生效" marker>
      <WmToolbar>
        <WmStatusPill :label="toolLabel" :tone="fw.tool === 'none' ? 'danger' : 'success'" />
        <a-switch v-if="fw.tool !== 'none'" v-model:checked="fw.enabled" @change="switchFw" />
        <a-button size="small" @click="load">刷新</a-button>
      </WmToolbar>
      <WmNotice v-if="fw.tool === 'none'" tone="warning" title="未检测到受支持的防火墙">
        {{ fw.message || '当前系统未检测到 ufw 或 firewalld，端口规则功能不可用。' }}
      </WmNotice>
      <template v-else>
        <WmNotice tone="warning" class="wm-mb-4">移除面板端口的放行规则可能导致失去远程访问，请先确认保留 SSH 与面板端口。</WmNotice>
        <WmToolbar>
          <a-input v-model:value="newPort" placeholder="如 8080/tcp 或 53/udp" style="width: 220px" @keyup.enter="addPort" />
          <a-button type="primary" @click="addPort">放行端口</a-button>
          <template #note>格式：端口或端口段 / 协议，例如 8080/tcp、8000-8100/tcp。</template>
        </WmToolbar>
        <WmTable :data="fw.ports" size="small" empty-title="暂无放行规则" empty-description="在上方输入端口与协议后添加规则。">
          <WmColumn label="端口/协议" min-width="180"><template #default="{ row }"><span class="wm-mono">{{ row }}</span></template></WmColumn>
          <WmColumn label="操作" width="110">
            <template #default="{ row }"><a-button size="small" data-tone="danger" @click="delPort(row)">移除</a-button></template>
          </WmColumn>
        </WmTable>
      </template>
    </WmSectionCard>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from '../antd-compat'
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
