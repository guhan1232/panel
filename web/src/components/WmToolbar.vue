<template>
  <div class="wm-toolbar">
    <div class="wm-toolbar__group"><slot /></div>
    <div v-if="$slots.end || $slots.note" class="wm-toolbar__group wm-toolbar__group--end">
      <slot name="end" />
      <span v-if="$slots.note" class="wm-toolbar__note"><slot name="note" /></span>
    </div>
  </div>
</template>
