import { defineStore } from 'pinia'

export const useTheme = defineStore('theme', {
  state: () => ({ dark: localStorage.getItem('wm_theme') === 'dark' }),
  actions: {
    apply() {
      document.documentElement.classList.toggle('dark', this.dark)
      localStorage.setItem('wm_theme', this.dark ? 'dark' : 'light')
    },
    toggle() {
      this.dark = !this.dark
      this.apply()
    }
  }
})
