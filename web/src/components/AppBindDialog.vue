<template>
  <a-modal :open="open" title="绑定 APP" :footer="null" @cancel="$emit('update:open', false)" width="430px">
    <div class="bind-wrap">
      <div class="bind-qr">
        <QrcodeVue v-if="qrText" :value="qrText" :size="200" level="M" render-as="svg" />
        <a-spin v-else size="large" />
      </div>
      <p class="bind-tip">打开寒语馨面板 APP → 扫一扫 → 自动添加服务器；在 APP 中使用面板账号登录即完成绑定。</p>
      <p v-if="info" class="bind-meta">
        服务器 <span class="wm-mono">{{ info.host }}</span>
        <br />入口码 <span class="wm-mono">{{ info.entrance }}</span>
      </p>
      <a-button size="small" @click="load">刷新二维码</a-button>
    </div>
  </a-modal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import QrcodeVue from 'vue-qrcode'
import api from '../api'

const props = defineProps<{ open: boolean }>()
defineEmits(['update:open'])

const info = ref<any>(null)
const qrText = ref('')

async function load() {
  qrText.value = ''
  try {
    const d: any = await api.get('/auth/app-bind')
    info.value = d
    qrText.value = d.qr_text
  } catch { /* 保持空态 */ }
}

watch(() => props.open, (v) => { if (v) load() })
</script>

<style scoped>
.bind-wrap { display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 6px 0 2px; }
.bind-qr { padding: 12px; border: 1px solid var(--wm-line); border-radius: 12px; background: #fff; line-height: 0; }
.bind-tip { margin: 0; font-size: 13px; color: var(--wm-ink-2); text-align: center; }
.bind-meta { margin: 0; font-size: 12px; color: var(--wm-ink-3); text-align: center; }
</style>
