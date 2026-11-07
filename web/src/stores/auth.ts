import { defineStore } from 'pinia'
import api from '../api'

export const useAuth = defineStore('auth', {
  state: () => ({ username: '', role: '', checked: false }),
  actions: {
    async fetchMe() {
      try {
        const d: any = await api.get('/auth/me')
        this.username = d?.username || ''
        this.role = d?.role || ''
      } catch {
        this.username = ''
        this.role = ''
      }
      this.checked = true
    },
    async login(username: string, password: string) {
      await api.post('/auth/login', { username, password })
      await this.fetchMe()
    },
    async logout() {
      try {
        await api.post('/auth/logout')
      } catch {
        /* ignore */
      }
      this.username = ''
      this.role = ''
    }
  }
})
