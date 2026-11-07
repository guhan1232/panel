<template>
  <header class="wm-page-header">
    <div class="wm-page-header__main">
      <div v-if="eyebrow" class="wm-eyebrow">{{ eyebrow }}</div>
      <h1 class="wm-page-header__title">{{ title }}</h1>
      <p v-if="description" class="wm-page-header__desc">{{ description }}</p>
    </div>
    <div v-if="$slots.actions || $slots.meta" class="wm-page-header__side">
      <div v-if="$slots.meta" class="wm-page-header__meta"><slot name="meta" /></div>
      <div v-if="$slots.actions" class="wm-page-header__actions"><slot name="actions" /></div>
    </div>
  </header>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    eyebrow?: string
    title: string
    description?: string
  }>(),
  { eyebrow: '', description: '' }
)
</script>
