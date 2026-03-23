import { createContext, useContext, useEffect, useReducer, type ReactNode } from 'react'
import { authApi } from '#/lib/api/auth'
import type { Account } from '#/lib/types/auth'

interface AuthState {
  user: Account | null
  isAuthenticated: boolean
  isLoading: boolean
}

type AuthAction =
  | { type: 'LOADING' }
  | { type: 'AUTHENTICATED'; user: Account }
  | { type: 'UNAUTHENTICATED' }

function authReducer(state: AuthState, action: AuthAction): AuthState {
  switch (action.type) {
    case 'LOADING':        return { ...state, isLoading: true }
    case 'AUTHENTICATED':  return { user: action.user, isAuthenticated: true, isLoading: false }
    case 'UNAUTHENTICATED': return { user: null, isAuthenticated: false, isLoading: false }
  }
}

interface AuthContextValue extends AuthState {
  login: (email: string, password: string) => Promise<void>
  logout: () => Promise<void>
}

const AuthContext = createContext<AuthContextValue | null>(null)

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(authReducer, {
    user: null, isAuthenticated: false, isLoading: true,
  })

  // On mount: attempt to restore session via httpOnly refresh cookie
  useEffect(() => {
    authApi.restoreSession()
      .then(user => {
        if (user) {
          dispatch({ type: 'AUTHENTICATED', user })
        } else {
          dispatch({ type: 'UNAUTHENTICATED' })
        }
      })
  }, [])

  const login = async (email: string, password: string) => {
    const user = await authApi.login(email, password)
    dispatch({ type: 'AUTHENTICATED', user })
  }

  const logout = async () => {
    await authApi.logout()
    dispatch({ type: 'UNAUTHENTICATED' })
  }

  return (
    <AuthContext.Provider value={{ ...state, login, logout }}>
      {children}
    </AuthContext.Provider>
  )
}

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) throw new Error('useAuth must be inside AuthProvider')
  return ctx
}
