<template>
  <span class="wm-pill" :class="[`wm-pill--${resolvedTone}`, { 'wm-pill--pulse': pulse, 'wm-pill--outline': outline, 'wm-pill--mono': mono }]">
    <i v-if="dot" class="wm-pill__dot" aria-hidden="true" />
    {{ label }}<slot />
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

type Tone = 'success' | 'warning' | 'danger' | 'info' | 'neutral' | 'muted'

const props = withDefaults(
  defineProps<{
    label: string
    tone?: Tone | ''
    dot?: boolean
    pulse?: boolean
    outline?: boolean
    mono?: boolean
  }>(),
  { tone: '', dot: true, pulse: false, outline: false, mono: false }
)

// Status vocabulary used across the panel. Unknown values fall back to a
// neutral pill rather than guessing a semantic color.
const MAP: Record<string, Tone> = {
  online: 'success', 在线: 'success', active: 'success', 正常: 'success',
  enabled: 'success', 已启用: 'success', 已连接: 'success', 已安装: 'success',
  就绪: 'success', https: 'success', success: 'success', 成功: 'success',
  运行中: 'info', running: 'info', 进行中: 'info', 已重载: 'info',
  offline: 'muted', 离线: 'muted', 未连接: 'muted', 未安装: 'muted',
  已停止: 'muted', stopped: 'muted', skipped: 'muted', 跳过: 'muted',
  failed: 'danger', 失败: 'danger', 已禁用: 'danger', 错误: 'danger',
  timeout: 'warning', 超时: 'warning', warning: 'warning', 告警: 'warning',
  即将到期: 'warning', 待处理: 'warning',
  info: 'info', primary: 'info', admin: 'danger', operator: 'info', viewer: 'neutral'
}

const resolvedTone = computed<Tone>(() => {
  if (props.tone) return props.tone
  return MAP[String(props.label).trim().toLowerCase()] || MAP[String(props.label).trim()] || 'neutral'
})
</script>
