import { createRouter, createWebHistory } from 'vue-router'
import { useAuth } from '../stores/auth'

/**
 * Routes, paths and navigation labels are protected contracts — unchanged.
 * `meta.group` / `meta.desc` only feed the shared PageHeader + breadcrumb.
 */
const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('../views/Login.vue'), meta: { title: '登录' } },
    {
      path: '/',
      component: () => import('../layouts/MainLayout.vue'),
      redirect: '/dashboard',
      children: [
        { path: 'dashboard', component: () => import('../views/Dashboard.vue'), meta: { title: '仪表盘', group: '概览', desc: '实时资源指标、运行概览与健康状态，每 3 秒自动刷新。' } },
        { path: 'website', component: () => import('../views/Website.vue'), meta: { title: '网站', group: '站点与服务', desc: '管理 Nginx 站点、域名绑定、SSL 与访问日志。' } },
        { path: 'databases', component: () => import('../views/Databases.vue'), meta: { title: '数据库', group: '站点与服务', desc: 'MySQL / PostgreSQL 连接配置、建库建用户与授权。' } },
        { path: 'firewall', component: () => import('../views/Firewall.vue'), meta: { title: '防火墙', group: '系统运维', desc: '统一抽象 ufw / firewalld，管理端口放行规则。' } },
        { path: 'certs', component: () => import('../views/Certs.vue'), meta: { title: 'SSL 证书', group: '系统运维', desc: '证书上传、Let’s Encrypt 签发与到期续期监控。' } },
        { path: 'cron', component: () => import('../views/Cron.vue'), meta: { title: '计划任务', group: '系统运维', desc: '内置调度引擎，六类任务与完整执行历史。' } },
        { path: 'files', component: () => import('../views/Files.vue'), meta: { title: '文件管理', group: '系统运维', desc: '浏览、编辑、上传下载、压缩解压与回收站删除。' } },
        { path: 'terminal', component: () => import('../views/Terminal.vue'), meta: { title: '终端', group: '站点与服务', desc: '基于登录会话鉴权的 Web 终端，操作全程审计。' } },
        { path: 'proc', component: () => import('../views/Proc.vue'), meta: { title: '进程管理', group: '系统运维', desc: '进程与 systemd 服务的实时状态和控制。' } },
        { path: 'nodes', component: () => import('../views/Nodes.vue'), meta: { title: '节点管理', group: '系统运维', desc: 'Agent 反连接入、远程指标与命令执行。' } },
        { path: 'audit', component: () => import('../views/Audit.vue'), meta: { title: '审计日志', group: '面板', desc: '全部写操作留痕，支持方法、路径与操作人筛选。' } },
        { path: 'alerts', component: () => import('../views/Alerts.vue'), meta: { title: '告警策略', group: '系统运维', desc: '阈值规则与触发历史，通知渠道在「通知与存储」配置。' } },
        { path: 'appstore', component: () => import('../views/Appstore.vue'), meta: { title: '应用商店', group: '站点与服务', desc: '容器与原生应用模板化部署、启停与镜像源管理。' } },
        { path: 'notify', component: () => import('../views/Notify.vue'), meta: { title: '通知与存储', group: '面板', desc: 'SMTP、IM Webhook 与 S3 兼容对象存储配置。' } },
        { path: 'users', component: () => import('../views/Users.vue'), meta: { title: '用户管理', group: '面板', desc: '三角色 RBAC、禁用与密码重置，含防自锁保护。', admin: true } },
        { path: 'setting', component: () => import('../views/Setting.vue'), meta: { title: '系统设置', group: '面板', desc: '面板运行信息、入口配置说明与管理员密码修改。' } }
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
  if (to.meta.admin && auth.role !== 'admin') return '/dashboard'
  return true
})

export default router
