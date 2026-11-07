import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 本地开发代理目标：面板端口为随机生成（10000-30000），通过环境变量指定，
// 例如 WM_PANEL_URL=http://127.0.0.1:24567 npm run dev（端口见 config.yaml 的 listen）
const panelTarget = process.env.WM_PANEL_URL || 'http://127.0.0.1:24443'

export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: { '/api': panelTarget }
  },
  build: {
    chunkSizeWarningLimit: 2000
  }
})
