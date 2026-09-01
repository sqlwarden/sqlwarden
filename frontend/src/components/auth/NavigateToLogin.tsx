import { useState } from 'react'
import { Navigate, useRouterState } from '@tanstack/react-router'
import { loginSearchFor } from '#/lib/auth/login-redirect'
import { useDesktopRuntime } from '#/lib/desktop/context'
import { isNativeDesktop } from '#/lib/desktop/runtime'

export function NavigateToLogin() {
  const desktop = useDesktopRuntime()
  const href = useRouterState({ select: (state) => state.location.href })
  // Keep the source location stable while the router commits and unmounts the
  // protected layout; otherwise the intermediate /login render can erase it.
  const [search] = useState(() => loginSearchFor(href))

  if (desktop.native || isNativeDesktop()) {
    return <Navigate to="/" replace />
  }

  return <Navigate to="/login" search={search} replace />
}
