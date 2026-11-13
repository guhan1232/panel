// useWideViewport —— 宽视口断点响应式（工作台竖排导航 ⇆ 顶部导航切换共用）。
// 取代各页手写的 window.innerWidth resize 监听。
import { onBeforeUnmount, onMounted, ref } from 'vue'

export function useWideViewport(breakpoint = 1100) {
  const wide = ref(window.innerWidth > breakpoint)
  let mql: MediaQueryList | null = null
  function onChange(e: MediaQueryListEvent | MediaQueryList) {
    wide.value = e.matches
  }
  onMounted(() => {
    mql = window.matchMedia(`(min-width: ${breakpoint + 1}px)`)
    onChange(mql)
    mql.addEventListener('change', onChange)
  })
  onBeforeUnmount(() => mql?.removeEventListener('change', onChange))
  return { wide }
}
