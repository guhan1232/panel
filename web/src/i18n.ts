// 轻量 i18n：菜单/登录页/通用操作双语字典（业务视图文案暂保持中文，逐步迁移）。
import { reactive } from 'vue'

type Dict = Record<string, string>

const dicts: Record<string, Dict> = {
  'zh-CN': {}, // 中文为源语言，键即中文
  en: {
    // 菜单
    '概览': 'Overview',
    '站点与服务': 'Sites & Services',
    '系统运维': 'Operations',
    '面板': 'Panel',
    '仪表盘': 'Dashboard',
    '网站': 'Websites',
    '数据库': 'Databases',
    '软件商店': 'Software Store',
    'Docker 容器管理': 'Docker',
    '终端': 'Terminal',
    '计划任务': 'Cron Jobs',
    '告警策略': 'Alerts',
    '文件管理': 'Files',
    '进程管理': 'Processes',
    '节点管理': 'Nodes',
    '防火墙': 'Firewall',
    'SSL 证书': 'SSL Certs',
    '审计日志': 'Audit Log',
    '通知与存储': 'Notify & Storage',
    '用户管理': 'Users',
    '系统设置': 'Settings',
    // 登录页
    '登录面板': 'Sign in',
    '使用管理员分配的账号继续': 'Sign in with the account assigned by your admin',
    '用户名': 'Username',
    '密码': 'Password',
    '登录': 'Sign in',
    '两因素验证码': '2FA code',
    '6 位动态码': '6-digit code',
    // 通用操作
    '启动': 'Start',
    '停止': 'Stop',
    '重启': 'Restart',
    '日志': 'Logs',
    '卸载': 'Uninstall',
    '安装': 'Install',
    '保存': 'Save',
    '取消': 'Cancel',
    '刷新': 'Refresh',
    '删除': 'Delete',
    '全部写操作均被审计': 'All write operations are audited'
  }
}

const state = reactive({ locale: localStorage.getItem('hanyuxin-locale') || 'zh-CN' })

export function setLocale(l: string) {
  state.locale = l
  localStorage.setItem('hanyuxin-locale', l)
}

export function getLocale() {
  return state.locale
}

// t 翻译：源语言为中文；en 字典缺失时回退中文。
export function t(key: string): string {
  if (state.locale === 'zh-CN') return key
  return dicts[state.locale]?.[key] ?? key
}