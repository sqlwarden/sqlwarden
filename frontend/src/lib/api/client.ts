import axios from 'axios'

// Token stored in module variable — never localStorage (XSS-safe)
let _accessToken: string | null = null
export const setAccessToken = (token: string | null) => { _accessToken = token }
export const getAccessToken = () => _accessToken

export const api = axios.create({
  baseURL: '/api/v1',
  withCredentials: true, // send httpOnly refresh cookie
})

// Inject auth header on every request
api.interceptors.request.use((config) => {
  const token = getAccessToken()
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

// On 401: attempt silent refresh, retry once, then redirect to login
let refreshing: Promise<string | null> | null = null

api.interceptors.response.use(
  r => r,
  async (error) => {
    const original = error.config
    if (error.response?.status === 401 && !original._retried) {
      original._retried = true
      if (!refreshing) {
        refreshing = axios
          .post<{ access_token: string }>('/api/v1/auth/refresh', {}, { withCredentials: true })
          .then(r => { setAccessToken(r.data.access_token); return r.data.access_token })
          .catch(() => { setAccessToken(null); return null })
          .finally(() => { refreshing = null })
      }
      const token = await refreshing
      if (token) {
        original.headers.Authorization = `Bearer ${token}`
        return api(original)
      }
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)
