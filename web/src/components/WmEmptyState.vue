<template>
  <div class="wm-empty" :class="[`wm-empty--${tone}`, { 'wm-empty--compact': compact }]">
    <div class="wm-empty__icon">
      <span class="wm-empty__icon">
        <WarningFilled v-if="tone === 'error'" />
        <Connection v-else-if="tone === 'offline'" />
        <Box v-else />
      </span>
    </div>
    <div class="wm-empty__title">{{ title }}</div>
    <div v-if="description || $slots.description" class="wm-empty__desc">
      <slot name="description">{{ description }}</slot>
    </div>
    <div v-if="$slots.action" class="wm-empty__action"><slot name="action" /></div>
  </div>
</template>

<script setup lang="ts">
import { Box, WarningFilled, Connection } from '../antd-compat'

withDefaults(
  defineProps<{
    title?: string
    description?: string
    compact?: boolean
    tone?: 'neutral' | 'error' | 'offline'
  }>(),
  { title: '暂无数据', description: '', compact: false, tone: 'neutral' }
)
</script>
