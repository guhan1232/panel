<template>
  <section
    class="wm-section"
    :class="{ 'wm-section--dense': dense, 'wm-section--quiet': quiet }"
  >
    <div v-if="title || description || $slots.actions" class="wm-section__head">
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
withDefaults(
  defineProps<{
    title?: string
    description?: string
    dense?: boolean
    flush?: boolean
    quiet?: boolean
    marker?: boolean
  }>(),
  { title: '', description: '', dense: false, flush: false, quiet: false, marker: false }
)
</script>
