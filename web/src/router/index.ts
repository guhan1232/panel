import { createRouter, createWebHistory } from 'vue-router'
import { useAuth } from '../stores/auth'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('../views/Login.vue') },
    {
      path: '/',
      component: () => import('../layouts/MainLayout.vue'),
      redirect: '/dashboard',
      children: [
        { path: 'dashboard', component: () => import('../views/Dashboard.vue'), meta: { title: '仪表盘' } },
        { path: 'cron', component: () => import('../views/Cron.vue'), meta: { title: '计划任务' } },
        { path: 'files', component: () => import('../views/Files.vue'), meta: { title: '文件管理' } },
        { path: 'terminal', component: () => import('../views/Terminal.vue'), meta: { title: '终端' } },
        { path: 'proc', component: () => import('../views/Proc.vue'), meta: { title: '进程管理' } },
        { path: 'setting', component: () => import('../views/Setting.vue'), meta: { title: '系统设置' } }
      ]
    },
    { path: '/:pathMatch(.*)*', redirect: '/' }
  ]
})

router.beforeEach(async (to) => {
  if (to.path === '/login') return true
  const auth = useAuth()
  if (!auth.checked) await auth.fetchMe()
  if (!auth.username) return '/login'
  return true
})

export default router
