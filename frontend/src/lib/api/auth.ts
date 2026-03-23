// src/lib/api/auth.ts
// Note: login and logout use `api` (authenticated axios instance).
// The session restore (refresh on mount) uses bare `axios` to bypass the auth interceptor
// since there is no access token yet — the refresh cookie is used instead.
// The `api` instance's refresh interceptor also uses bare `axios` for the same reason.
import axios from 'axios'
import { api, setAccessToken } from '#/lib/api/client'
import type { Account } from '#/lib/types/auth'

export const authApi = {
  /**
   * Exchange email + password for an access token. Sets the access token in memory.
   * Returns the current account.
   */
  login: async (email: string, password: string): Promise<Account> => {
    const res = await api.post<{ access_token: string }>('/auth/login', { email, password })
    setAccessToken(res.data.access_token)
    const userRes = await api.get<Account>('/user')
    return userRes.data
  },

  /**
   * Attempt to restore a session from the httpOnly refresh cookie.
   * Uses bare axios to bypass the auth interceptor (no access token yet).
   * Returns the current account, or null if no valid session.
   */
  restoreSession: async (): Promise<Account | null> => {
    try {
      const refreshRes = await axios.post<{ access_token: string }>(
        '/api/v1/auth/refresh',
        {},
        { withCredentials: true },
      )
      setAccessToken(refreshRes.data.access_token)
      const userRes = await api.get<Account>('/user')
      return userRes.data
    } catch {
      return null
    }
  },

  /**
   * Log out the current user. Clears the access token from memory.
   */
  logout: async (): Promise<void> => {
    await api.post('/auth/logout').catch(() => {})
    setAccessToken(null)
  },
}
