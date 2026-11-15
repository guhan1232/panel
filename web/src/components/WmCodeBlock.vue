<template>
  <div class="wm-code">
    <div class="wm-code__bar">
      <span class="wm-truncate">{{ label }}</span>
      <button class="wm-code__copy" type="button" :disabled="!content" @click="copy">
        {{ copied ? '已复制' : '复制' }}
      </button>
    </div>
    <pre ref="preEl" class="wm-code__pre"><span v-if="content">{{ content }}</span><span v-else class="wm-code__empty">{{ emptyText }}</span></pre>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { ElMessage } from '../antd-compat'

withDefaults(
  defineProps<{
    content?: string
    label?: string
    emptyText?: string
  }>(),
  { content: '', label: 'output', emptyText: '（无输出）' }
)

const copied = ref(false)
const preEl = ref<HTMLElement>()
let reset: number | null = null

async function copy() {
  const text = preEl.value?.textContent || ''
  try {
    await navigator.clipboard.writeText(text)
    copied.value = true
    if (reset) window.clearTimeout(reset)
    reset = window.setTimeout(() => (copied.value = false), 1600)
  } catch {
    ElMessage.warning('浏览器未允许复制，请手动选择文本')
  }
}
</script>
