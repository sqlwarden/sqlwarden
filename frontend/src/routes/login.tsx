import { errorMessage } from '#/lib/api/errors'
import { useState } from 'react'
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
import { AuthField } from '#/components/auth/AuthField'
import { AuthLayout } from '#/components/auth/AuthLayout'
import { AuthProviderList } from '#/components/auth/AuthProviderList'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { PasswordInput } from '#/components/ui/password-input'

export const Route = createFileRoute('/login')({
  component: LoginPage,
})

function LoginPage() {
  const navigate = useNavigate()
  const redirect = safeRedirect(new URLSearchParams(window.location.search).get('redirect'))
  const queryClient = useQueryClient()
  const setupStatus = useSetupStatus()
  const [values, setValues] = useState({ email: '', password: '' })
  const [localErrors, setLocalErrors] = useState<Record<string, string>>({})
  const hasToken = Boolean(getAccessToken())
  const session = useSession(hasToken)

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
        window.location.replace(redirect)
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
      window.location.replace(redirect)
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
    <AuthLayout
      title="Sign in"
      description="Enter your credentials to access your organizations."
      footer={
        <p className="text-center text-xs text-muted-foreground">
          Trouble signing in? Contact your instance administrator.
        </p>
      }
    >
      <form className="space-y-5" onSubmit={onSubmit}>
        <AuthField label="Email address" error={formErrors.email}>
          <Input
            autoComplete="email"
            type="email"
            value={values.email}
            onChange={(event) => {
              setValues((current) => ({ ...current, email: event.target.value }))
              setLocalErrors((current) => {
                const next = { ...current }
                delete next.email
                return next
              })
            }}
          />
        </AuthField>

        <AuthField label="Password" error={formErrors.password}>
          <PasswordInput
            autoComplete="current-password"
            value={values.password}
            onChange={(event) => {
              setValues((current) => ({ ...current, password: event.target.value }))
              setLocalErrors((current) => {
                const next = { ...current }
                delete next.password
                return next
              })
            }}
          />
        </AuthField>
        <Button className="h-10 w-full" size="lg" disabled={mutation.isPending} type="submit">
          {mutation.isPending ? 'Signing in…' : 'Sign in'}
        </Button>
      </form>

      <AuthProviderList redirect={redirect} />
    </AuthLayout>
  )
}

function safeRedirect(value: string | null) {
  return value && value.startsWith('/') && !value.startsWith('//') ? value : undefined
}
