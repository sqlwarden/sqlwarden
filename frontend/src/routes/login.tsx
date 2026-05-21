import { useMemo, useState } from 'react'
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
import { Badge } from '#/components/ui/badge'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { Input } from '#/components/ui/input'

export const Route = createFileRoute('/login')({
  component: LoginPage,
})

function LoginPage() {
  const navigate = useNavigate()
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
      await navigate({ to: '/account', replace: true })
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        return
      }

      toast.error(error instanceof Error ? error.message : 'Failed to sign in')
    },
  })

  const serverFieldErrors = isApiError(mutation.error) ? mutation.error.fieldErrors ?? {} : {}
  const formErrors = useMemo(
    () => ({ ...serverFieldErrors, ...localErrors }),
    [localErrors, serverFieldErrors],
  )

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
    return <Navigate to="/account" replace />
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
    <main className="flex min-h-screen items-center justify-center px-4 py-12">
      <div className="w-full max-w-md space-y-6">
        <div className="space-y-3 text-center">
          <Badge variant="outline">SQLWarden</Badge>
          <div className="space-y-2">
            <h1 className="text-2xl font-semibold tracking-tight">Sign in</h1>
            <p className="text-sm text-muted-foreground">
              Use the instance admin account you just created.
            </p>
          </div>
        </div>

        <Card className="py-0">
          <CardHeader className="px-6 pt-6">
            <CardTitle>Account login</CardTitle>
            <CardDescription>
              Enter your credentials to continue.
            </CardDescription>
          </CardHeader>
          <CardContent className="px-6 pb-6">
            <form className="space-y-5" onSubmit={onSubmit}>
              <Field label="Email address" error={formErrors.email}>
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
              </Field>

              <Field label="Password" error={formErrors.password}>
                <Input
                  autoComplete="current-password"
                  type="password"
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
              </Field>
              <Button className="h-10 w-full" size="lg" disabled={mutation.isPending} type="submit">
                {mutation.isPending ? 'Signing in…' : 'Sign in'}
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </main>
  )
}

function Field({
  children,
  error,
  label,
}: {
  children: React.ReactNode
  error?: string
  label: string
}) {
  return (
    <div className="space-y-2">
      <label className="text-sm font-medium">{label}</label>
      {children}
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </div>
  )
}
