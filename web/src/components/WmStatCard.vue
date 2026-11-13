<template>
  <div class="wm-stat" :class="[`wm-stat--${tone}`, { 'wm-stat--dense': dense }]">
    <div class="wm-stat__top">
      <span class="wm-stat__label">{{ label }}</span>
      <span
        v-if="$slots.icon || icon"
        class="wm-stat__icon"
        :style="palette ? { background: palette.soft, color: palette.color } : undefined"
      >
        <slot name="icon"><span v-if="icon" class="wm-stat__icon-inline"><component :is="icon" /></span></slot>
      </span>
    </div>

    <template v-if="loading">
      <div class="wm-stat__skeleton" style="width: 62%; height: 30px; margin-top: 14px" />
      <div class="wm-stat__skeleton" style="width: 86%; height: 11px; margin-top: 12px" />
    </template>
    <template v-else>
      <div class="wm-stat__value" :class="{ 'wm-stat__value--text': text }">
        <slot name="value">{{ value }}<span v-if="unit" class="wm-stat__unit">{{ unit }}</span></slot>
      </div>
      <div v-if="sub || $slots.sub" class="wm-stat__sub"><slot name="sub">{{ sub }}</slot></div>
      <div v-if="progress != null" class="wm-stat__progress">
        <div class="wm-stat__bar" role="progressbar" :aria-valuenow="Math.round(progress)" aria-valuemin="0" aria-valuemax="100">
          <i :style="{ width: `${Math.max(0, Math.min(100, progress))}%` }" />
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    label: string
    value?: string | number
    unit?: string
    sub?: string
    tone?: 'info' | 'success' | 'warning' | 'danger' | 'neutral'
    progress?: number | null
    text?: boolean
    loading?: boolean
    icon?: any
    dense?: boolean
    /** 彩色模块色板变量名对，覆盖 tone 的图标底色 */
    palette?: { color: string; soft: string } | null
  }>(),
  { value: '-', unit: '', sub: '', tone: 'info', progress: null, text: false, loading: false, icon: null, dense: false, palette: null }
)
void props
</script>
