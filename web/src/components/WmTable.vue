<template>
  <div class="wm-table-wrap">
    <a-table
      v-bind="restAttrs"
      class="wm-table"
      :class="{ 'wm-table--compact': compact }"
      :columns="columns"
      :data-source="rows"
      :row-key="rowKey"
      :row-selection="rowSelection"
      :pagination="false"
      :scroll="scroll"
      :size="tableSize"
    >
      <template #emptyText>
        <slot name="empty">
          <WmEmptyState
            compact
            :tone="emptyTone"
            :title="emptyTitle"
            :description="emptyDescription"
          />
        </slot>
      </template>
    </a-table>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, useAttrs, useSlots, type VNode } from 'vue'
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

const attrs = useAttrs() as Record<string, any>
const slots = useSlots()

/** Flatten fragments / v-for arrays so conditional columns still register. */
function isColumnVNode(n: any): boolean {
  if (!n?.type || typeof n.type !== 'object') return false
  const p = n.props || {}
  return (
    p.label !== undefined || p.prop !== undefined || p.type !== undefined ||
    p['min-width'] !== undefined || p.minWidth !== undefined || p.width !== undefined
  )
}
function flatten(nodes: any[]): VNode[] {
  const out: VNode[] = []
  for (const n of nodes || []) {
    if (!n || typeof n !== 'object') continue
    if (isColumnVNode(n)) {
      out.push(n)
      continue
    }
    const kids = n.children
    if (Array.isArray(kids)) out.push(...flatten(kids))
  }
  return out
}

function toNumber(v: any): number | undefined {
  if (v == null || v === '') return undefined
  const n = Number(v)
  return Number.isFinite(n) ? n : undefined
}

const columnNodes = computed<VNode[]>(() => flatten(slots.default?.() || []))

const columns = computed(() =>
  columnNodes.value
    .filter((n) => (n.props || {}).type !== 'selection')
    .map((n, i) => {
      const p = n.props || {}
      const children = n.children as any
      const render = children && typeof children.default === 'function' ? children.default : null
      const dataIndex = p.prop || p.dataIndex
      return {
        title: p.label || p.title || '',
        dataIndex,
        key: dataIndex || `col-${i}`,
        width: toNumber(p.width),
        minWidth: toNumber(p.minWidth || p['min-width']),
        fixed: p.fixed === 'right' ? ('right' as const) : p.fixed === 'left' ? ('left' as const) : undefined,
        align: p.align,
        ellipsis: !!p['show-overflow-tooltip'],
        customRender: render
          ? ({ record, index }: any) => render({ row: record, $index: index, column: { property: dataIndex } })
          : undefined
      } as any
    })
)

const rawRows = computed<any[]>(() => {
  const d = attrs['data-source'] ?? attrs.data ?? []
  return Array.isArray(d) ? d : []
})
const rows = computed(() => rawRows.value.map((r: any, i: number) => ({ ...r, __wmKey: r?.id ?? r?.key ?? i })))
const rowKey = (record: any) => record.__wmKey

const tableSize = computed<'small' | 'middle' | 'large'>(() =>
  attrs.size === 'large' ? 'large' : attrs.size === 'middle' ? 'middle' : 'small'
)

const scroll = computed(() => {
  const y = toNumber(attrs['max-height'] ?? attrs.height)
  const x = columnNodes.value.reduce((sum: number, n) => sum + (toNumber((n.props || {}).width) || 0), 0)
  return { x: x > 900 ? x : undefined, y }
})

const hasSelection = computed(() => columnNodes.value.some((n) => (n.props || {}).type === 'selection'))
const selectedKeys = ref<Array<string | number>>([])
const rowSelection = computed(() =>
  hasSelection.value
    ? {
        columnWidth: 44,
        selectedRowKeys: selectedKeys.value,
        onChange: (keys: Array<string | number>, selectedRows: any[]) => {
          selectedKeys.value = keys
          const handler = attrs.onSelectionChange
          if (typeof handler === 'function') handler(selectedRows)
        }
      }
    : undefined
)

const restAttrs = computed(() => {
  const skip = new Set([
    'data', 'data-source', 'size', 'max-height', 'height',
    'onSelectionChange', 'row-class-name', 'header-cell-style', 'class', 'style'
  ])
  const out: Record<string, any> = {}
  for (const [k, v] of Object.entries(attrs)) if (!skip.has(k)) out[k] = v
  return out
})
</script>
