// 品牌信息：自定义面板名称（后端 settings 表 panel_name；空 = 默认「寒语馨面板」）。
// 登录页与布局壳各自 onMounted 调 load()；GET /api/setting/brand 匿名可读（受安全入口防护）。
import { ref } from 'vue'
import api from '../api'

export const DEFAULT_PANEL_NAME = '寒语馨面板'

const name = ref(localStorage.getItem('wm_panel_name') || DEFAULT_PANEL_NAME)
const loaded = ref(false)

export function useBrand() {
  async function load() {
    try {
      const d: any = await api.get('/setting/brand')
      name.value = (d.name || '').trim() || DEFAULT_PANEL_NAME
      localStorage.setItem('wm_panel_name', name.value)
      document.title = `${name.value} · 服务器管理面板`
      loaded.value = true
    } catch {
      /* 网络异常时保留本地缓存值 */
    }
  }
  return { name, loaded, load, DEFAULT_PANEL_NAME }
}
