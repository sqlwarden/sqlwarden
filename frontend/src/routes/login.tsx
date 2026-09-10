import { errorMessage } from '#/lib/api/errors'
import { useEffect, useState } from 'react'
import { Navigate, createFileRoute, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { useSession } from '#/hooks/use-session'
import { api } from '#/lib/api/client'
import type { AccessTokenResponse } from '#/lib/api/types'
import { isApiError } from '#/lib/api/errors'
import { getAccessToken, setAccessToken } from '#/lib/auth/access-token'
import { clearAuthScopedQueryCache } from '#/lib/auth/query-cache'
import { AmbientBackground } from '#/components/auth/AmbientBackground'
import { LoginForm } from '#/components/auth/LoginForm'
import { LoginSurface } from '#/components/auth/LoginSurface'
import { usePageTitle } from '#/lib/page-title'
import { parseLoginSearch } from '#/lib/auth/login-redirect'
import { useDesktopRuntime } from '#/lib/desktop/context'
import { isNativeDesktop } from '#/lib/desktop/runtime'

export const Route = createFileRoute('/login')({
  validateSearch: parseLoginSearch,
  component: LoginPage,
})

function LoginPage() {
  usePageTitle('Login')
  const navigate = useNavigate()
  const { redirect } = Route.useSearch()
  const queryClient = useQueryClient()
  const desktop = useDesktopRuntime()
  const setupStatus = useSetupStatus()
  const [values, setValues] = useState({ email: '', password: '' })
  const [localErrors, setLocalErrors] = useState<Record<string, string>>({})
  const hasToken = Boolean(getAccessToken())
  const session = useSession(hasToken)

  useEffect(() => {
    if (hasToken && session.data && redirect) {
      void navigate({ href: redirect, replace: true })
    }
  }, [hasToken, navigate, redirect, session.data])

  const mutation = useMutation({
    mutationFn: async () =>
      api.post<AccessTokenResponse>(
        '/api/v1/auth/login',
        {
          email: values.email.trim(),
          password: values.password,
        },
        { skipAuth: true },
      ),
    onSuccess: async (payload) => {
      clearAuthScopedQueryCache(queryClient)
      setAccessToken(payload.access_token)
      if (redirect) {
        await navigate({ href: redirect, replace: true })
        return
      }
      await navigate({ to: '/', replace: true })
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        return
      }

      toast.error(errorMessage(error, 'Failed to sign in'))
    },
  })

  const formErrors = {
    ...(isApiError(mutation.error) ? (mutation.error.fieldErrors ?? {}) : {}),
    ...localErrors,
  }

  if (desktop.native || isNativeDesktop()) {
    return <Navigate to="/" replace />
  }

  if (setupStatus.isLoading || (hasToken && session.isLoading)) {
    return (
      <main className="flex min-h-screen items-center justify-center px-4">
        <div className="text-sm text-muted-foreground">Loading…</div>
      </main>
    )
  }

  if (setupStatus.data && !setupStatus.data.configured) {
    return <Navigate to="/setup" replace />
  }

  if (hasToken && session.data) {
    if (redirect) {
      return null
    }
    return <Navigate to="/" replace />
  }

  function validate() {
    const nextErrors: Record<string, string> = {}
    if (!values.email.trim()) nextErrors.email = 'Email is required.'
    if (!values.password) nextErrors.password = 'Password is required.'
    setLocalErrors(nextErrors)
    return Object.keys(nextErrors).length === 0
  }

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!validate()) {
      return
    }
    try {
      await mutation.mutateAsync()
    } catch {
      // handled by mutation onError
    }
  }

  return (
    <main className="relative">
      <AmbientBackground />
      <LoginSurface
        title="Sign in"
        description="Enter your credentials to access your organizations."
        footer={
          <p className="text-center text-xs text-muted-foreground">
            Trouble signing in? Contact your instance administrator.
          </p>
        }
      >
        <LoginForm
          email={values.email}
          password={values.password}
          emailError={formErrors.email}
          passwordError={formErrors.password}
          isPending={mutation.isPending}
          redirect={redirect}
          onEmailChange={(value) => {
            setValues((current) => ({ ...current, email: value }))
            setLocalErrors((current) => {
              const next = { ...current }
              delete next.email
              return next
            })
          }}
          onPasswordChange={(value) => {
            setValues((current) => ({ ...current, password: value }))
            setLocalErrors((current) => {
              const next = { ...current }
              delete next.password
              return next
            })
          }}
          onSubmit={onSubmit}
        />
      </LoginSurface>
    </main>
  )
}
