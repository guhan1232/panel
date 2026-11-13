<template>
  <section
    class="wm-section"
    :class="{ 'wm-section--dense': dense, 'wm-section--quiet': quiet }"
  >
    <div v-if="title || description || $slots.actions" class="wm-section__head">
      <span
        v-if="icon"
        class="wm-section__icon"
        :style="{ background: iconSoft, color: iconColor }"
        aria-hidden="true"
      >
        <component :is="icon" />
      </span>
      <div class="wm-section__head-text">
        <h2 v-if="title" class="wm-section__title">
          <i v-if="marker" class="wm-section__marker" aria-hidden="true" />{{ title }}
        </h2>
        <p v-if="description" class="wm-section__desc">{{ description }}</p>
        <slot name="head-extra" />
      </div>
      <div v-if="$slots.actions" class="wm-section__actions"><slot name="actions" /></div>
    </div>
    <div class="wm-section__body" :class="{ 'wm-section__body--flush': flush }">
      <slot />
    </div>
    <div v-if="$slots.footer" class="wm-section__foot"><slot name="footer" /></div>
  </section>
</template>

<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    title?: string
    description?: string
    dense?: boolean
    flush?: boolean
    quiet?: boolean
    marker?: boolean
    icon?: any
    /** 彩色模块色板变量名对，如 { color: 'var(--wm-p-green)', soft: 'var(--wm-p-green-soft)' } */
    palette?: { color: string; soft: string } | null
  }>(),
  { title: '', description: '', dense: false, flush: false, quiet: false, marker: false, icon: null, palette: null }
)
const iconColor = props.palette?.color || 'var(--wm-primary)'
const iconSoft = props.palette?.soft || 'var(--wm-primary-soft)'
</script>
