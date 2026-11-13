<template>
  <a-config-provider :theme="antTheme" :locale="zhCN">
    <router-view />
  </a-config-provider>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { theme } from 'ant-design-vue'
import zhCN from 'ant-design-vue/es/locale/zh_CN'
import { useTheme } from './stores/theme'

const store = useTheme()

const antTheme = computed(() => ({
  algorithm: store.dark ? theme.darkAlgorithm : theme.defaultAlgorithm,
  token: {
    colorPrimary: '#0f766e',
    colorInfo: '#0f766e',
    colorSuccess: '#059669',
    colorWarning: '#d97706',
    colorError: '#dc2626',
    colorLink: '#0d9488',
    borderRadius: 6,
    borderRadiusLG: 8,
    borderRadiusSM: 4,
    controlHeight: 36,
    fontSize: 14,
    fontFamily:
      '"Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", "Noto Sans CJK SC", sans-serif'
  }
}))
</script>
