import { createApp } from 'vue'
import { createPinia } from 'pinia'
import Antd from 'ant-design-vue'
import 'ant-design-vue/dist/reset.css'
import App from './App.vue'
import router from './router'
import { useTheme } from './stores/theme'
import { loadingDirective } from './antd-compat'
import * as WmComponents from './components'
import './style.css'

const app = createApp(App)
app.use(createPinia())
useTheme().apply()

// Ant Design Vue is the only UI runtime now; Element Plus and its CSS are gone.
app.use(Antd)
app.directive('loading', loadingDirective)

Object.entries(WmComponents).forEach(([name, component]) => app.component(name, component))

app.use(router).mount('#app')
