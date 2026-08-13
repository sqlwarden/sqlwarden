import { useState } from 'react'
import { Navigate, useRouterState } from '@tanstack/react-router'
import { loginSearchFor } from '#/lib/auth/login-redirect'

export function NavigateToLogin() {
  const href = useRouterState({ select: (state) => state.location.href })
  // Keep the source location stable while the router commits and unmounts the
  // protected layout; otherwise the intermediate /login render can erase it.
  const [search] = useState(() => loginSearchFor(href))
  return <Navigate to="/login" search={search} replace />
}
