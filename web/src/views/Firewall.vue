<template>
  <div class="wm-page">
    <WmSectionCard title="防火墙状态" description="统一抽象 ufw 与 firewalld，规则由面板托管（导入/导出/同步）" marker>
      <template #actions>
        <span v-if="fw.tool !== 'none'" class="wm-text-muted fw-stats">{{ rules.length }} 条规则 · 防扫描{{ fw.anti_scan ? '已开启' : '已关闭' }}</span>
      </template>
      <WmToolbar>
        <WmStatusPill :label="toolLabel" :tone="fw.tool === 'none' ? 'danger' : fw.enabled ? 'success' : 'warning'" :pulse="fw.enabled" />
        <a-switch v-if="fw.tool !== 'none'" v-model:checked="fw.enabled" @change="switchFw" />
        <template #end>
          <template v-if="fw.tool !== 'none'">
            <a-dropdown>
              <a-button size="small">更多<span class="wm-icon fw-dd-icon"><ArrowDown /></span></a-button>
              <template #overlay>
                <a-menu>
                  <a-menu-item key="import" @click="importDlg = true">导入规则</a-menu-item>
                  <a-menu-item key="export" @click="exportRules">导出规则</a-menu-item>
                  <a-menu-item key="sync" :disabled="syncing" @click="syncRules">同步规则</a-menu-item>
                  <a-menu-divider />
                  <a-menu-item key="antiscan" @click="toggleAntiScan">{{ fw.anti_scan ? '关闭端口防扫描' : '开启端口防扫描' }}</a-menu-item>
                </a-menu>
              </template>
            </a-dropdown>
            <a-button type="primary" size="small" @click="openAdd">添加端口规则</a-button>
          </template>
          <a-button size="small" @click="load">刷新</a-button>
        </template>
        <template #note>{{ fw.tool === 'ufw' ? 'ufw（Debian/Ubuntu）' : fw.tool === 'firewalld' ? 'firewalld（RHEL）' : fw.message || '' }}</template>
      </WmToolbar>

      <!-- 未安装引导 -->
      <WmNotice v-if="fw.tool === 'none'" tone="warning" title="未检测到受支持的防火墙">
        {{ fw.message || '当前系统未检测到 ufw 或 firewalld。' }}
        <div class="wm-mt-4">
          <a-button type="primary" :loading="installing" @click="installFw">一键安装防火墙（默认安装）</a-button>
          <p class="wm-text-muted" style="margin-top:8px">自动识别发行版：Debian/Ubuntu 安装 ufw，RHEL 系安装 firewalld。启用前会自动放行 SSH(22/tcp) 与面板端口，防止锁死远程访问。</p>
        </div>
      </WmNotice>

      <!-- 规则管理 -->
      <template v-else>
        <WmNotice tone="warning" class="wm-mb-4">移除 SSH 或面板端口的放行规则可能导致失去远程访问，请谨慎操作。</WmNotice>

        <WmTable :data="rules" v-loading="loading" size="small" empty-title="暂无端口规则" empty-description="点击「添加端口规则」创建第一条规则。">
          <template #empty>
            <div class="fw-empty">
              <div class="fw-empty__icon"><span class="wm-icon"><Lock /></span></div>
              <p class="fw-empty__title">暂无端口规则</p>
              <p class="fw-empty__desc">点击「添加端口规则」创建第一条规则；启用前会自动放行 SSH 与面板端口，防止锁死远程访问。</p>
              <a-button type="primary" @click="openAdd">添加端口规则</a-button>
            </div>
          </template>
          <WmColumn label="协议" width="90"><template #default="{ row }"><span class="wm-mono">{{ row.protocol.toUpperCase() }}</span></template></WmColumn>
          <WmColumn label="端口" min-width="150"><template #default="{ row }"><b class="wm-mono">{{ row.ports }}</b></template></WmColumn>
          <WmColumn label="来源" min-width="150"><template #default="{ row }"><span class="wm-mono">{{ row.source || '所有IP' }}</span></template></WmColumn>
          <WmColumn label="策略" width="100"><template #default="{ row }"><WmStatusPill :label="row.action === 'accept' ? '放行' : '阻止'" :tone="row.action === 'accept' ? 'success' : 'danger'" :dot="false" /></template></WmColumn>
          <WmColumn label="方向" width="90"><template #default="{ row }">{{ row.direction === 'out' ? '出站' : '入站' }}</template></WmColumn>
          <WmColumn label="备注" min-width="180"><template #default="{ row }"><span class="wm-text-muted">{{ row.remark || '-' }}<a-tag v-if="row.auto" style="margin-left:6px" size="small">系统</a-tag></span></template></WmColumn>
          <WmColumn label="操作" width="72" fixed="right">
            <template #default="{ row }"><a-button type="link" size="small" danger @click="delRule(row)">删除</a-button></template>
          </WmColumn>
        </WmTable>
      </template>
    </WmSectionCard>

    <!-- 添加端口规则 -->
    <a-drawer v-model:open="addDlg" title="添加端口规则" :width="620">
      <a-form layout="vertical">
        <a-form-item label="协议">
          <a-radio-group v-model:value="form.protocol" option-type="button" button-style="solid">
            <a-radio-button value="tcp">TCP</a-radio-button>
            <a-radio-button value="udp">UDP</a-radio-button>
          </a-radio-group>
        </a-form-item>
        <a-form-item label="端口" required>
          <a-input v-model:value="form.ports" placeholder="请输入端口，如 80 或 80,88,90-99" />
        </a-form-item>
        <a-form-item label="来源">
          <a-select v-model:value="form.sourceMode">
            <a-select-option value="all">所有IP</a-select-option>
            <a-select-option value="custom">指定IP/网段</a-select-option>
          </a-select>
          <a-input v-if="form.sourceMode === 'custom'" v-model:value="form.source" placeholder="如 10.0.0.1 或 192.168.1.0/24" class="wm-mt-2" />
        </a-form-item>
        <a-form-item label="策略">
          <a-radio-group v-model:value="form.action" option-type="button" button-style="solid">
            <a-radio-button value="accept">放行</a-radio-button>
            <a-radio-button value="deny">阻止</a-radio-button>
          </a-radio-group>
        </a-form-item>
        <a-form-item label="方向">
          <a-select v-model:value="form.direction">
            <a-select-option value="in">入站(默认)</a-select-option>
            <a-select-option value="out">出站</a-select-option>
          </a-select>
        </a-form-item>
        <a-form-item label="备注" style="margin-bottom: 0"><a-input v-model:value="form.remark" placeholder="请填写备注，可为空" /></a-form-item>
      </a-form>
      <ul class="rule-hints">
        <li>支持添加多个端口，如：<code>80,88</code></li>
        <li>支持添加多个端口范围，如：<code>80,88,90-99,110-120</code></li>
        <li>默认配置下防火墙无法管理 Docker 容器端口</li>
        <li>此操作将重载防火墙，未持久化的 iptables 规则会清空，请先备份保存</li>
      </ul>
      <template #footer>
        <a-button @click="addDlg = false">取消</a-button>
        <a-button type="primary" @click="submitAdd">确定</a-button>
      </template>
    </a-drawer>

    <!-- 导入规则 -->
    <a-modal v-model:open="importDlg" title="导入规则（JSON）" width="640px" ok-text="导入" cancel-text="取消" @ok="doImport">
      <p class="wm-text-muted">粘贴「导出规则」生成的 JSON 数组，或自行按字段构造：protocol / ports / source / action(accept|deny) / direction(in|out) / remark。</p>
      <a-textarea v-model:value="importText" :rows="10" class="wm-mono" placeholder='[{"protocol":"tcp","ports":"80,443","source":"","action":"accept","direction":"in","remark":"web"}]' />
    </a-modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox, ArrowDown, Lock } from '../antd-compat'
import api from '../api'

const fw = ref<any>({ tool: 'none', enabled: false, anti_scan: false, ports: [], message: '' })
const rules = ref<any[]>([])
const loading = ref(false)
const installing = ref(false)
const syncing = ref(false)

const addDlg = ref(false)
const form = reactive({ protocol: 'tcp', ports: '', sourceMode: 'all', source: '', action: 'accept', direction: 'in', remark: '' })
const importDlg = ref(false)
const importText = ref('')

const toolLabel = computed(() => ({ ufw: 'ufw 已启用', firewalld: 'firewalld 运行中', none: '未检测到防火墙' } as any)[fw.value.tool] || fw.value.tool)

async function load() {
  fw.value = { ...fw.value, ...(await api.get('/firewall/status')) }
  if (fw.value.tool !== 'none') {
    rules.value = (await api.get('/firewall/rules')) as any
  }
}

function openAdd() {
  Object.assign(form, { protocol: 'tcp', ports: '', sourceMode: 'all', source: '', action: 'accept', direction: 'in', remark: '' })
  addDlg.value = true
}

async function submitAdd() {
  if (!form.ports.trim()) {
    ElMessage.warning('请输入端口')
    return
  }
  if (form.sourceMode === 'custom' && !form.source.trim()) {
    ElMessage.warning('请填写来源 IP/网段')
    return
  }
  await api.post('/firewall/rule', {
    protocol: form.protocol,
    ports: form.ports.trim(),
    source: form.sourceMode === 'custom' ? form.source.trim() : '',
    action: form.action,
    direction: form.direction,
    remark: form.remark.trim()
  })
  ElMessage.success('规则已添加并生效')
  addDlg.value = false
  load()
}

async function delRule(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除规则「${row.protocol.toUpperCase()} ${row.ports}」？`, '删除规则', { type: 'warning' })
  } catch {
    return
  }
  await api.delete('/firewall/rule/' + row.id)
  ElMessage.success('已删除')
  load()
}

async function doImport() {
  let list: any
  try {
    list = JSON.parse(importText.value)
  } catch {
    ElMessage.warning('JSON 格式不正确')
    return
  }
  if (!Array.isArray(list) || !list.length) {
    ElMessage.warning('内容应为非空 JSON 数组')
    return
  }
  const r: any = await api.post('/firewall/rules/import', { rules: list }, { timeout: 300000 })
  ElMessage.success(r?.message || '导入完成')
  importDlg.value = false
  importText.value = ''
  load()
}

function exportRules() {
  const blob = new Blob([JSON.stringify(rules.value.map((r: any) => ({
    protocol: r.protocol, ports: r.ports, source: r.source, action: r.action, direction: r.direction, remark: r.remark
  })), null, 2)], { type: 'application/json' })
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = 'hanyuxin-firewall-rules.json'
  a.click()
  URL.revokeObjectURL(a.href)
}

async function syncRules() {
  try {
    await ElMessageBox.confirm('将面板托管的全部规则重新下发到防火墙（防自锁端口自动保留），继续？', '同步规则', { type: 'warning' })
  } catch {
    return
  }
  syncing.value = true
  try {
    const r: any = await api.post('/firewall/sync', {}, { timeout: 600000 })
    ElMessage.success(r?.message || '同步完成')
    load()
  } finally {
    syncing.value = false
  }
}

async function toggleAntiScan() {
  const enable = !fw.value.anti_scan
  const r: any = await api.post('/firewall/anti-scan', { enabled: enable }, { timeout: 300000 })
  ElMessage.success(r?.message || (enable ? '防扫描已开启' : '防扫描已关闭'))
  load()
}

async function switchFw() {
  await api.post('/firewall/switch', { enabled: fw.value.enabled })
  ElMessage.success(fw.value.enabled ? '防火墙已启用' : '防火墙已关闭')
}

async function installFw() {
  installing.value = true
  try {
    const r: any = await api.post('/firewall/install', {}, { timeout: 620000 })
    ElMessage.success(r?.message || '安装完成')
    await load()
  } finally {
    installing.value = false
  }
}

onMounted(load)
</script>

<style scoped>
.fw-stats { font-size: 12px; }
.fw-dd-icon { margin-left: 4px; font-size: 10px; }

/* 空态 */
.fw-empty {
  display: flex; flex-direction: column; align-items: center; text-align: center;
  padding: 48px 20px 52px;
}
.fw-empty__icon {
  width: 44px; height: 44px; border-radius: 50%;
  background: var(--wm-surface-3); color: var(--wm-ink-3);
  display: grid; place-items: center; font-size: 20px; margin-bottom: 12px;
}
.fw-empty__title { font-size: 14px; font-weight: 500; color: var(--wm-ink); }
.fw-empty__desc { font-size: 12px; color: var(--wm-ink-3); margin: 4px 0 14px; max-width: 46ch; line-height: 1.7; }

/* 规则表单提示 */
.rule-hints {
  margin: 16px 0 0;
  padding-left: 18px;
  color: var(--wm-ink-3);
  font-size: 12px;
  line-height: 1.9;
}
.rule-hints code {
  background: var(--wm-surface-3);
  color: var(--wm-ink-2);
  padding: 1px 6px;
  border-radius: var(--wm-r-sm);
  font-family: var(--wm-font-mono);
}
</style>
