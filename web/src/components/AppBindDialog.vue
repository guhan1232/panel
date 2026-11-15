<template>
  <a-modal :open="open" title="绑定 APP" :footer="null" @cancel="$emit('update:open', false)" width="460px">
    <div class="bind-wrap">
      <!-- 未生成密钥 -->
      <template v-if="!st.enabled">
        <p class="bind-tip">生成绑定密钥后，用 APP 扫码或手动输入即可绑定（免账号密码，直接登录）。</p>
        <a-button type="primary" :loading="busy" @click="generate">生成绑定密钥</a-button>
      </template>
      <template v-else-if="st.bound && rebinding">
        <div class="bind-qr">
          <QrcodeVue v-if="qrText" :value="qrText" :size="200" level="M" render-as="svg" />
        </div>
        <p class="bind-tip">已生成新密钥并吊销旧设备会话——用 APP 重新扫码即绑定到新设备。</p>
      </template>

      <!-- 已生成：显示二维码 + 轮询绑定状态 -->
      <template v-else>
        <div class="bind-qr">
          <QrcodeVue v-if="qrText" :value="qrText" :size="200" level="M" render-as="svg" />
          <a-spin v-else size="large" />
        </div>
        <p v-if="!st.bound" class="bind-tip">打开寒语馨面板 APP → 扫一扫，自动绑定并登录面板<br />（密钥长期有效，绑定后此码不再变化）</p>

        <!-- 已绑定：自动回调完成，不再刷新二维码 -->
        <a-result v-if="st.bound && !rebinding" status="success" title="绑定成功" :sub-title="`已绑定设备：${st.device_name || 'APP'} · 绑定于 ${st.bound_at || ''}`" style="padding: 12px 0" />
        <template v-if="st.bound && !rebinding">
          <a-space wrap style="justify-content: center">
            <a-popconfirm title="将解绑当前设备并吊销其会话，确定？" @confirm="doUnbind">
              <a-button danger :loading="busy">取消绑定</a-button>
            </a-popconfirm>
            <a-button :loading="busy" @click="startRebind">重新绑定设备</a-button>
          </a-space>
          <p class="bind-tip">重新绑定会生成新密钥并吊销旧设备会话，用 APP 重新扫码即可切换到新设备。</p>
        </template>

        <p class="bind-key-line">绑定密钥 <span class="wm-mono">{{ keyMask }}</span>
          <a-button v-if="st.enabled && !revealed" type="link" size="small" @click="revealed = true">显示</a-button>
        </p>
        <p v-if="revealed && keyFull" class="bind-key-full wm-mono">{{ keyFull }}</p>

        <a-button size="small" @click="load">刷新状态</a-button>
        <a-alert v-if="st.bound" type="success" show-icon message="APP 已绑定本面板，打开 APP 即可免密使用" class="bind-ok" />
      </template>
    </div>
  </a-modal>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import QrcodeVue from 'vue-qrcode'
import api from '../api'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits(['update:open'])

const st = ref<any>({ enabled: false, bound: false })
const bindInfo = ref<any>(null)
const keyFull = ref('')
const rebinding = ref(false)
const revealed = ref(false)
const busy = ref(false)
let poll: ReturnType<typeof setInterval> | undefined

const qrText = computed(() => {
  if (!bindInfo.value || !keyFull.value) return ''
  return `hanyuxin://bind?v=1&host=${encodeURIComponent(bindInfo.value.host)}&entrance=${encodeURIComponent(bindInfo.value.entrance)}&key=${keyFull.value}&name=${encodeURIComponent(bindInfo.value.host)}`
})
const keyMask = computed(() => keyFull.value ? keyFull.value.slice(0, 8) + '••••••••' : '—')

async function load() {
  try {
    st.value = (await api.get('/auth/bind/status')) as any
  } catch { /* 保持 */ }
}

async function loadBindInfo() {
  try {
    bindInfo.value = (await api.get('/auth/app-bind')) as any
  } catch { /* 保持 */ }
}

async function doUnbind() {
  busy.value = true
  try {
    await api.post('/auth/bind/unbind')
    keyFull.value = ''
    revealed.value = false
    await load()
  } finally {
    busy.value = false
  }
}

async function startRebind() {
  rebinding.value = true
  busy.value = true
  try {
    const d: any = await api.post('/auth/bind/generate')
    keyFull.value = d.key
    revealed.value = true
    await load()
  } finally {
    busy.value = false
  }
}

async function generate() {
  busy.value = true
  try {
    const d: any = await api.post('/auth/bind/generate')
    keyFull.value = d.key
    revealed.value = true
    await load()
    loadBindInfo()
  } finally {
    busy.value = false
  }
}

async function unbind() {
  busy.value = true
  try {
    await api.post('/auth/bind/unbind')
    keyFull.value = ''
    revealed.value = false
    await load()
  } finally {
    busy.value = false
  }
}

defineExpose({ generate, unbind })

watch(() => props.open, (v) => {
  if (v) {
    load()
    loadBindInfo()
    // 绑定状态轮询（2s）：bound=true 时自动回调完成
    poll = setInterval(async () => {
      try {
        const d: any = await api.get('/auth/bind/status')
        st.value = d
        if (d.bound) {
          clearInterval(poll)
          poll = undefined
          rebinding.value = false
        }
      } catch { /* 忽略瞬时错误 */ }
    }, 2000)
  } else if (poll) {
    clearInterval(poll)
    poll = undefined
  }
})
onUnmounted(() => { if (poll) clearInterval(poll) })
</script>

<style scoped>
.bind-wrap { display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 6px 0 2px; }
.bind-qr { padding: 12px; border: 1px solid var(--wm-line); border-radius: 12px; background: #fff; line-height: 0; }
.bind-tip { margin: 0; font-size: 13px; color: var(--wm-ink-2); text-align: center; }
.bind-key-line { margin: 0; font-size: 12px; color: var(--wm-ink-3); }
.bind-key-full { margin: 0; font-size: 11px; word-break: break-all; background: var(--wm-surface-2); padding: 6px 8px; border-radius: 6px; }
.bind-ok { width: 100%; }
</style>
