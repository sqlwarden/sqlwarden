import { errorMessage } from '#/lib/api/errors'
import { useState } from 'react'
import { Navigate, createFileRoute, useNavigate } from '@tanstack/react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { isApiError } from '#/lib/api/errors'
import { api } from '#/lib/api/client'
import type { SetupResponse } from '#/lib/api/types'
import { clearAccessToken } from '#/lib/auth/access-token'
import { queryKeys } from '#/lib/api/query'
import { MAX_SLUG_LENGTH, slugify } from '#/lib/strings'
import { AmbientBackground } from '#/components/auth/AmbientBackground'
import { AuthField } from '#/components/auth/AuthField'
import { LoginSurface } from '#/components/auth/LoginSurface'
import { Badge } from '#/components/ui/badge'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { PasswordInput } from '#/components/ui/password-input'
import { Icon } from '#/lib/icons'
import { usePageTitle } from '#/lib/page-title'

export const Route = createFileRoute('/setup')({
  component: SetupPage,
})

function SetupPage() {
  usePageTitle('Setup')
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const setupStatus = useSetupStatus()
  const [values, setValues] = useState({
    name: '',
    email: '',
    password: '',
    confirmPassword: '',
    organizationName: '',
    organizationSlug: '',
  })
  const [slugTouched, setSlugTouched] = useState(false)
  const [localErrors, setLocalErrors] = useState<Record<string, string>>({})

  const mutation = useMutation({
    mutationFn: async () =>
      api.post<SetupResponse>('/api/setup', setupPayload(values), {
        skipAuth: true,
      }),
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

      toast.error(errorMessage(error, 'Failed to complete setup'))
    },
  })

  const formErrors = {
    ...(isApiError(mutation.error) ? (mutation.error.fieldErrors ?? {}) : {}),
    ...localErrors,
  }

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
    setValues((current) => {
      const next = { ...current, [field]: value }
      if (field === 'organizationName' && !slugTouched) {
        next.organizationSlug = slugify(value, { maxLength: MAX_SLUG_LENGTH })
      }
      return next
    })
    setLocalErrors((current) => {
      const next = { ...current }
      delete next[field]
      if (field === 'organizationName') {
        delete next.organization_name
        if (!slugTouched) delete next.organization_slug
      }
      if (field === 'organizationSlug') {
        delete next.organization_slug
      }
      if (field === 'password' || field === 'confirmPassword') {
        delete next.password
        delete next.confirmPassword
      }
      return next
    })
  }

  function validate() {
    const nextErrors: Record<string, string> = {}

    if (!values.name.trim()) nextErrors.name = 'Name is required.'
    if (!values.email.trim()) nextErrors.email = 'Email is required.'
    if (!values.organizationName.trim())
      nextErrors.organization_name = 'Organization name is required.'
    if (!values.organizationSlug.trim())
      nextErrors.organization_slug = 'Organization slug is required.'
    else if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(values.organizationSlug.trim())) {
      nextErrors.organization_slug =
        'Organization slug may only contain lowercase letters, numbers, and hyphens.'
    }
    if (!values.password) nextErrors.password = 'Password is required.'
    else if (values.password.length < 8)
      nextErrors.password = 'Password must be at least 8 characters.'
    if (!values.confirmPassword) nextErrors.confirmPassword = 'Please confirm the password.'
    else if (values.password !== values.confirmPassword)
      nextErrors.confirmPassword = 'Passwords do not match.'

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
        eyebrow={
          <Badge variant="outline" className="mb-1">
            First-time setup
          </Badge>
        }
        title="Set up SQLWarden"
        description="Create an administrator account and organization to get started."
        className="max-w-[480px]"
        footer={
          <p className="text-center text-xs text-muted-foreground">
            No users exist yet. This account becomes the instance administrator.
          </p>
        }
      >
        <form className="space-y-5" onSubmit={onSubmit}>
          <AuthField label="Full name" error={formErrors.name}>
            <Input
              autoComplete="name"
              placeholder="Alex Ward"
              value={values.name}
              onChange={(event) => updateField('name', event.target.value)}
            />
          </AuthField>

          <AuthField label="Email address" error={formErrors.email}>
            <Input
              autoComplete="email"
              type="email"
              placeholder="admin@organization.com"
              value={values.email}
              onChange={(event) => updateField('email', event.target.value)}
            />
          </AuthField>

          <div className="grid gap-5 sm:grid-cols-2">
            <AuthField label="Organization name" error={formErrors.organization_name}>
              <Input
                autoComplete="organization"
                placeholder="Acme Cloud"
                value={values.organizationName}
                onChange={(event) => updateField('organizationName', event.target.value)}
              />
            </AuthField>

            <AuthField label="Organization slug" error={formErrors.organization_slug}>
              <Input
                autoComplete="off"
                maxLength={MAX_SLUG_LENGTH}
                placeholder="acme-cloud"
                value={values.organizationSlug}
                onChange={(event) => {
                  setSlugTouched(true)
                  updateField(
                    'organizationSlug',
                    slugify(event.target.value, { maxLength: MAX_SLUG_LENGTH }),
                  )
                }}
              />
            </AuthField>
          </div>

          <div className="grid gap-5 sm:grid-cols-2">
            <AuthField label="Password" error={formErrors.password}>
              <PasswordInput
                autoComplete="new-password"
                placeholder="Minimum 8 characters"
                value={values.password}
                onChange={(event) => updateField('password', event.target.value)}
              />
            </AuthField>

            <AuthField label="Confirm password" error={formErrors.confirmPassword}>
              <PasswordInput
                autoComplete="new-password"
                placeholder="Repeat password"
                value={values.confirmPassword}
                onChange={(event) => updateField('confirmPassword', event.target.value)}
              />
            </AuthField>
          </div>

          <div className="rounded-lg border bg-muted/40 p-4 text-sm text-muted-foreground">
            This account gets instance admin access and becomes the owner of the first organization.
          </div>

          <Button
            className="h-11 w-full rounded-lg text-sm shadow-md transition-all duration-150 ease-out hover:shadow-lg active:translate-y-px active:scale-[0.98] active:shadow-sm active:duration-75"
            size="lg"
            disabled={mutation.isPending}
            type="submit"
          >
            {mutation.isPending ? (
              <span className="flex items-center gap-2">
                <Icon name="loading-03" size={14} className="animate-spin" />
                Setting up…
              </span>
            ) : (
              'Create admin and organization'
            )}
          </Button>
        </form>
      </LoginSurface>
    </main>
  )
}

function setupPayload(values: {
  name: string
  email: string
  password: string
  organizationName: string
  organizationSlug: string
}) {
  const payload: Record<string, string> = {
    name: values.name.trim(),
    email: values.email.trim(),
    password: values.password,
  }

  payload.organization_name = values.organizationName.trim()
  payload.organization_slug = values.organizationSlug.trim()

  return payload
}
