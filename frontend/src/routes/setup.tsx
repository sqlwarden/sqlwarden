import { useMemo, useState } from 'react'
import { Navigate, createFileRoute, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { isApiError } from '#/lib/api/errors'
import { api } from '#/lib/api/client'
import type { SetupResponse } from '#/lib/api/types'
import { clearAccessToken } from '#/lib/auth/access-token'
import { queryKeys } from '#/lib/api/query'
import { Badge } from '#/components/ui/badge'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { Input } from '#/components/ui/input'

export const Route = createFileRoute('/setup')({
  component: SetupPage,
})

function SetupPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const setupStatus = useSetupStatus()
  const [values, setValues] = useState({
    name: '',
    email: '',
    password: '',
    confirmPassword: '',
  })
  const [localErrors, setLocalErrors] = useState<Record<string, string>>({})

  const mutation = useMutation({
    mutationFn: async () =>
      api.post<SetupResponse>(
        '/api/setup',
        {
          name: values.name.trim(),
          email: values.email.trim(),
          password: values.password,
        },
        { skipAuth: true },
      ),
    onSuccess: async (payload) => {
      void payload
      clearAccessToken()
      await queryClient.invalidateQueries({ queryKey: queryKeys.setupStatus() })
      await navigate({ to: '/login', replace: true })
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        return
      }

      toast.error(error instanceof Error ? error.message : 'Failed to complete setup')
    },
  })

  const serverFieldErrors = isApiError(mutation.error) ? mutation.error.fieldErrors ?? {} : {}
  const formErrors = useMemo(
    () => ({ ...serverFieldErrors, ...localErrors }),
    [localErrors, serverFieldErrors],
  )

  if (setupStatus.isLoading) {
    return (
      <main className="flex min-h-screen items-center justify-center px-4">
        <div className="text-sm text-muted-foreground">Loading setup state…</div>
      </main>
    )
  }

  if (setupStatus.data?.configured) {
    return <Navigate to="/" replace />
  }

  function updateField<K extends keyof typeof values>(field: K, value: (typeof values)[K]) {
    setValues((current) => ({ ...current, [field]: value }))
    setLocalErrors((current) => {
      const next = { ...current }
      delete next[field]
      if (field === 'password' || field === 'confirmPassword') {
        delete next.password
        delete next.confirmPassword
      }
      return next
    })
  }

  function validate() {
    const nextErrors: Record<string, string> = {}

    if (!values.name.trim()) nextErrors.name = 'Name is required'
    if (!values.email.trim()) nextErrors.email = 'Email is required'
    if (!values.password) nextErrors.password = 'Password is required'
    else if (values.password.length < 8) nextErrors.password = 'Password must be at least 8 characters'
    if (!values.confirmPassword) nextErrors.confirmPassword = 'Please confirm the password'
    else if (values.password !== values.confirmPassword) nextErrors.confirmPassword = 'Passwords do not match'

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
      <div className="w-full max-w-lg space-y-6">
        <div className="space-y-3 text-center">
          <Badge variant="outline">First-time setup</Badge>
          <div className="space-y-2">
            <h1 className="text-2xl font-semibold tracking-tight">Create the instance admin</h1>
            <p className="text-sm text-muted-foreground">
              This account will administer the SQLWarden instance. Organization setup comes next.
            </p>
          </div>
        </div>

        <Card className="py-0">
          <CardHeader className="px-6 pt-6">
            <CardTitle>Instance Setup</CardTitle>
            <CardDescription>
              Configure the primary administrative identity for this deployment.
            </CardDescription>
          </CardHeader>

          <CardContent className="px-6 pb-6">
            <form className="space-y-5" onSubmit={onSubmit}>
              <Field label="Full name" error={formErrors.name}>
                <Input
                  autoComplete="name"
                  placeholder="Alex Ward"
                  value={values.name}
                  onChange={(event) => updateField('name', event.target.value)}
                />
              </Field>

              <Field label="Email address" error={formErrors.email}>
                <Input
                  autoComplete="email"
                  type="email"
                  placeholder="admin@organization.com"
                  value={values.email}
                  onChange={(event) => updateField('email', event.target.value)}
                />
              </Field>

              <div className="grid gap-5 sm:grid-cols-2">
                <Field label="Password" error={formErrors.password}>
                  <Input
                    autoComplete="new-password"
                    type="password"
                    placeholder="Minimum 8 characters"
                    value={values.password}
                    onChange={(event) => updateField('password', event.target.value)}
                  />
                </Field>

                <Field label="Confirm password" error={formErrors.confirmPassword}>
                  <Input
                    autoComplete="new-password"
                    type="password"
                    placeholder="Repeat password"
                    value={values.confirmPassword}
                    onChange={(event) => updateField('confirmPassword', event.target.value)}
                  />
                </Field>
              </div>

              <div className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">
                This account gets full instance-level access. Organization creation can be skipped later.
              </div>
              <Button className="h-10 w-full" disabled={mutation.isPending} size="lg" type="submit">
                {mutation.isPending ? 'Creating admin account…' : 'Continue to sign in'}
              </Button>
            </form>
          </CardContent>
        </Card>

        <p className="text-center text-xs text-muted-foreground">
          No users exist yet. Complete this step to bootstrap the instance.
        </p>
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
