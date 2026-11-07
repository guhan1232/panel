<template>
  <div class="wm-table-wrap">
    <el-table
      v-bind="$attrs"
      class="wm-table"
      :class="{ 'wm-table--compact': compact }"
      :header-cell-style="{ background: 'var(--wm-surface-2)' }"
    >
      <template v-for="name in forwarded" :key="name" #[name]="scope">
        <slot :name="name" v-bind="scope || {}" />
      </template>
      <template #empty>
        <slot name="empty">
          <WmEmptyState
            compact
            :tone="emptyTone"
            :title="emptyTitle"
            :description="emptyDescription"
          />
        </slot>
      </template>
    </el-table>
  </div>
</template>

<script setup lang="ts">
import { computed, useSlots } from 'vue'
import WmEmptyState from './WmEmptyState.vue'

defineOptions({ inheritAttrs: false })

withDefaults(
  defineProps<{
    compact?: boolean
    emptyTitle?: string
    emptyDescription?: string
    emptyTone?: 'neutral' | 'error' | 'offline'
  }>(),
  { compact: true, emptyTitle: '暂无数据', emptyDescription: '', emptyTone: 'neutral' }
)

const slots = useSlots()
const forwarded = computed(() => Object.keys(slots).filter((k) => k !== 'empty'))
</script>
