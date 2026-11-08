<script setup lang="ts">
/**
 * Declarative column descriptor. It renders nothing by itself; WmTable reads
 * its props and scoped slot from the default slot vnodes and turns them into
 * ant-design-vue `columns`. This keeps the existing view templates intact
 * while the actual table is rendered by a-table.
 */
defineOptions({ inheritAttrs: false })
</script>

<template>
  <span style="display: none" aria-hidden="true"></span>
</template>
