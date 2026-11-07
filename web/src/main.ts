import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import App from './App.vue'
import router from './router'
import { useTheme } from './stores/theme'
import './style.css'

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
useTheme().apply()
app.use(router).use(ElementPlus, { locale: zhCn }).mount('#app')
