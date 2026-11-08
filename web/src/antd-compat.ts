/**
 * Ant Design Vue compatibility surface.
 *
 * The business views were originally written against Element Plus's
 * imperative APIs (`ElMessage`, `ElMessageBox`) and its icon names. Keeping
 * those call signatures here means every view's <script setup> logic stays
 * byte-for-byte intact while the rendered UI is entirely ant-design-vue.
 */
import { h, ref } from 'vue'
import { message, Modal, Input } from 'ant-design-vue'
import {
  DashboardOutlined, GlobalOutlined, DatabaseOutlined, AppstoreOutlined,
  CodeOutlined, ClockCircleOutlined, AlertOutlined, FolderOutlined,
  ThunderboltOutlined, HddOutlined, LockOutlined, SafetyCertificateOutlined,
  FileTextOutlined, BellOutlined, SettingOutlined, MenuFoldOutlined,
  MenuUnfoldOutlined, BulbOutlined, SkinOutlined, DownOutlined, LogoutOutlined,
  UserOutlined, ShareAltOutlined, MenuOutlined, MoreOutlined, InboxOutlined,
  DeleteOutlined, LineChartOutlined, DesktopOutlined, CloseOutlined,
  InfoCircleFilled, CheckCircleFilled, ExclamationCircleFilled
} from '@ant-design/icons-vue'

export const ElMessage = {
  success: (text: string) => message.success(text),
  error: (text: string) => message.error(text),
  warning: (text: string) => message.warning(text),
  info: (text: string) => message.info(text)
}

type ConfirmOptions = {
  type?: string
  confirmButtonText?: string
  cancelButtonText?: string
}

type PromptOptions = {
  inputValue?: string
  inputType?: string
  inputPlaceholder?: string
}

function confirm(msg: string, title = '确认', options: ConfirmOptions = {}) {
  return new Promise<string>((resolve, reject) => {
    Modal.confirm({
      title,
      content: msg,
      okText: options.confirmButtonText || '确定',
      cancelText: options.cancelButtonText || '取消',
      okType: options.type === 'error' ? 'danger' : 'primary',
      onOk: () => resolve('confirm'),
      onCancel: () => reject('cancel')
    })
  })
}

function prompt(msg: string, title = '输入', options: PromptOptions = {}) {
  return new Promise<{ value: string }>((resolve, reject) => {
    const value = ref(options.inputValue || '')
    const Content = () =>
      h('div', { style: 'display:flex;flex-direction:column;gap:8px;padding:4px 0 2px' }, [
        h('div', { style: 'color:var(--wm-ink-2,#595959);font-size:13px' }, msg),
        h(Input, {
          value: value.value,
          'onUpdate:value': (v: string) => (value.value = v),
          type: options.inputType === 'password' ? 'password' : 'text',
          placeholder: options.inputPlaceholder || '',
          autofocus: true,
          onPressEnter: () => {}
        })
      ])
    Modal.confirm({
      title,
      content: h(Content),
      okText: '确定',
      cancelText: '取消',
      onOk: () => resolve({ value: value.value }),
      onCancel: () => reject('cancel')
    })
  })
}

export const ElMessageBox = { confirm, prompt }

/* Icon aliases: Element Plus names -> Ant Design Icons (same component API). */
export {
  DashboardOutlined as Odometer,
  GlobalOutlined as Connection,
  DatabaseOutlined as Coin,
  AppstoreOutlined as Shop,
  CodeOutlined as Monitor,
  ClockCircleOutlined as Timer,
  AlertOutlined as Warning,
  FolderOutlined as Folder,
  ThunderboltOutlined as Cpu,
  HddOutlined as Memo,
  LineChartOutlined as TrendCharts,
  DesktopOutlined as Platform,
  LockOutlined as Lock,
  SafetyCertificateOutlined as Stamp,
  FileTextOutlined as Document,
  BellOutlined as Bell,
  SettingOutlined as Setting,
  MenuUnfoldOutlined as Expand,
  MenuFoldOutlined as Fold,
  BulbOutlined as Sunny,
  SkinOutlined as Moon,
  DownOutlined as ArrowDown,
  LogoutOutlined as SwitchButton,
  UserOutlined as Avatar,
  UserOutlined as User,
  ShareAltOutlined as Share,
  MenuOutlined as Menu,
  MoreOutlined as MoreFilled,
  InboxOutlined as Box,
  DeleteOutlined as Delete,
  CloseOutlined as Close,
  InfoCircleFilled as InfoFilled,
  ExclamationCircleFilled as WarningFilled,
  CheckCircleFilled as CircleCheckFilled
}

/** Lightweight `v-loading` directive replacement (no Element Plus runtime). */
export const loadingDirective = {
  mounted(el: HTMLElement, binding: any) {
    el.classList.add('wm-loading-host')
    syncLoading(el, !!binding.value)
  },
  updated(el: HTMLElement, binding: any) {
    syncLoading(el, !!binding.value)
  },
  unmounted(el: HTMLElement) {
    el.querySelector('.wm-loading-mask')?.remove()
  }
}

function syncLoading(el: HTMLElement, active: boolean) {
  const existing = el.querySelector('.wm-loading-mask')
  if (active && !existing) {
    const mask = document.createElement('div')
    mask.className = 'wm-loading-mask'
    mask.innerHTML = '<span class="wm-loading-spin"></span>'
    el.appendChild(mask)
  } else if (!active && existing) {
    existing.remove()
  }
}
