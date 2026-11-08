<template>
  <transition name="fade">
    <div v-if="visible" class="wm-notice" :class="`wm-notice--${tone}`" role="status">
      <span class="wm-notice__icon">
        <CircleCheckFilled v-if="tone === 'success'" />
        <WarningFilled v-else-if="tone === 'warning' || tone === 'danger'" />
        <InfoFilled v-else />
      </span>
      <div class="wm-notice__body">
        <strong v-if="title" class="wm-notice__title">{{ title }}</strong>
        <slot />
      </div>
      <div v-if="$slots.action" class="wm-notice__action"><slot name="action" /></div>
      <span v-if="closable" class="wm-notice__close" @click="visible = false"><Close /></span>
    </div>
  </transition>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { InfoFilled, WarningFilled, CircleCheckFilled, Close } from '../antd-compat'

withDefaults(
  defineProps<{
    title?: string
    tone?: 'info' | 'success' | 'warning' | 'danger' | 'neutral'
    closable?: boolean
  }>(),
  { title: '', tone: 'info', closable: false }
)

const visible = ref(true)
</script>
