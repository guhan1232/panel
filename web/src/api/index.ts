import axios from 'axios'
import { ElMessage } from '../antd-compat'
import router from '../router'

const api = axios.create({ baseURL: '/api', timeout: 30000 })

api.interceptors.response.use(
  (resp) => {
    const d = resp.data
    if (d && typeof d === 'object' && 'code' in d) {
      if (d.code !== 0) {
        ElMessage.error(d.msg || '操作失败')
        return Promise.reject(d)
      }
      return d.data
    }
    return d
  },
  (err) => {
    if (err.response?.status === 401) {
      router.push('/login')
    } else {
      ElMessage.error(err.response?.data?.msg || err.message || '网络错误')
    }
    return Promise.reject(err)
  }
)

export default api
