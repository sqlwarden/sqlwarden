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
import { cn } from '#/lib/utils'
import { AmbientBackground } from '#/components/auth/AmbientBackground'
import { AuthField } from '#/components/auth/AuthField'
import { LoginSurface } from '#/components/auth/LoginSurface'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { PasswordInput } from '#/components/ui/password-input'
import { Icon } from '#/lib/icons'
import { usePageTitle } from '#/lib/page-title'

export const Route = createFileRoute('/setup')({
  component: SetupPage,
})

const ACCOUNT_FIELD_KEYS = ['name', 'email', 'password', 'confirmPassword'] as const

const fieldInputClass = 'h-10 px-3 text-sm md:text-sm'
const fieldPasswordInputClass = 'h-10 pl-3 pr-9 text-sm md:text-sm'

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
  const [step, setStep] = useState<1 | 2>(1)

  const mutation = useMutation({
    mutationFn: async () =>
      api.post<SetupResponse>('/api/setup', setupPayload(values, setupStatus.data?.access_mode), {
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
        const errorKeys = Object.keys(error.fieldErrors)
        if (ACCOUNT_FIELD_KEYS.some((key) => errorKeys.includes(key))) {
          setStep(1)
        }
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

  const requiresOrganization = setupStatus.data?.access_mode !== 'single_user'
  const onAccountStep = !requiresOrganization || step === 1

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

  function validateAccountFields() {
    const nextErrors: Record<string, string> = {}

    if (!values.name.trim()) nextErrors.name = 'Name is required.'
    if (!values.email.trim()) nextErrors.email = 'Email is required.'
    if (!values.password) nextErrors.password = 'Password is required.'
    else if (values.password.length < 8)
      nextErrors.password = 'Password must be at least 8 characters.'
    if (!values.confirmPassword) nextErrors.confirmPassword = 'Please confirm the password.'
    else if (values.password !== values.confirmPassword)
      nextErrors.confirmPassword = 'Passwords do not match.'

    setLocalErrors((current) => ({ ...current, ...nextErrors }))
    return Object.keys(nextErrors).length === 0
  }

  function validateOrganizationFields() {
    const nextErrors: Record<string, string> = {}

    if (!values.organizationName.trim())
      nextErrors.organization_name = 'Organization name is required.'
    if (!values.organizationSlug.trim())
      nextErrors.organization_slug = 'Organization slug is required.'
    else if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(values.organizationSlug.trim())) {
      nextErrors.organization_slug =
        'Organization slug may only contain lowercase letters, numbers, and hyphens.'
    }

    setLocalErrors((current) => ({ ...current, ...nextErrors }))
    return Object.keys(nextErrors).length === 0
  }

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()

    if (!validateAccountFields()) {
      setStep(1)
      return
    }

    if (requiresOrganization && step === 1) {
      setStep(2)
      return
    }

    if (requiresOrganization && !validateOrganizationFields()) {
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
          requiresOrganization ? (
            <div className="mb-1 flex flex-col items-center gap-2.5">
              <StepProgress step={step} />
            </div>
          ) : undefined
        }
        title={stepTitle(requiresOrganization, step)}
        description={stepDescription(requiresOrganization, step)}
        className="max-w-[420px]"
      >
        <form className="space-y-5" onSubmit={onSubmit}>
          {onAccountStep ? (
            <>
              <AuthField label="Full name" error={formErrors.name}>
                <Input
                  className={fieldInputClass}
                  autoComplete="name"
                  placeholder="Alex Ward"
                  value={values.name}
                  onChange={(event) => updateField('name', event.target.value)}
                />
              </AuthField>

              <AuthField label="Email address" error={formErrors.email}>
                <Input
                  className={fieldInputClass}
                  autoComplete="email"
                  type="email"
                  placeholder="admin@organization.com"
                  value={values.email}
                  onChange={(event) => updateField('email', event.target.value)}
                />
              </AuthField>

              <AuthField label="Password" error={formErrors.password}>
                <PasswordInput
                  className={fieldPasswordInputClass}
                  autoComplete="new-password"
                  placeholder="Minimum 8 characters"
                  value={values.password}
                  onChange={(event) => updateField('password', event.target.value)}
                />
              </AuthField>

              <AuthField label="Confirm password" error={formErrors.confirmPassword}>
                <PasswordInput
                  className={fieldPasswordInputClass}
                  autoComplete="new-password"
                  placeholder="Repeat password"
                  value={values.confirmPassword}
                  onChange={(event) => updateField('confirmPassword', event.target.value)}
                />
              </AuthField>
            </>
          ) : (
            <>
              <AuthField label="Organization name" error={formErrors.organization_name}>
                <Input
                  className={fieldInputClass}
                  autoComplete="organization"
                  placeholder="Acme Cloud"
                  value={values.organizationName}
                  onChange={(event) => updateField('organizationName', event.target.value)}
                />
              </AuthField>

              <AuthField label="Organization slug" error={formErrors.organization_slug}>
                <Input
                  className={fieldInputClass}
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
            </>
          )}

          <div className={cn('flex gap-3', !onAccountStep && 'flex-row-reverse')}>
            <Button
              className="h-10 flex-1 rounded-lg text-sm"
              size="lg"
              disabled={mutation.isPending}
              type="submit"
            >
              {mutation.isPending ? (
                <span className="flex items-center gap-2">
                  <Icon name="loading-03" size={14} className="animate-spin" />
                  Setting up…
                </span>
              ) : requiresOrganization ? (
                'Continue'
              ) : (
                'Create admin account'
              )}
            </Button>

            {!onAccountStep ? (
              <Button
                className="h-10 rounded-lg text-sm"
                size="lg"
                type="button"
                variant="outline"
                disabled={mutation.isPending}
                onClick={() => setStep(1)}
              >
                Back
              </Button>
            ) : null}
          </div>
        </form>
      </LoginSurface>
    </main>
  )
}

function StepProgress({ step }: { step: 1 | 2 }) {
  return (
    <div className="flex items-center gap-1.5" aria-hidden="true">
      {([1, 2] as const).map((segment) => (
        <span
          key={segment}
          className={cn('h-1 w-8 rounded-full', segment <= step ? 'bg-primary' : 'bg-muted')}
        />
      ))}
    </div>
  )
}

function setupPayload(
  values: {
    name: string
    email: string
    password: string
    organizationName: string
    organizationSlug: string
  },
  accessMode: 'multi_user' | 'single_user' | undefined,
) {
  const payload: Record<string, string> = {
    name: values.name.trim(),
    email: values.email.trim(),
    password: values.password,
  }

  if (accessMode !== 'single_user') {
    payload.organization_name = values.organizationName.trim()
    payload.organization_slug = values.organizationSlug.trim()
  }

  return payload
}

function stepTitle(requiresOrganization: boolean, step: 1 | 2) {
  if (!requiresOrganization) return 'Set up SQLWarden'
  return step === 1 ? 'Create your administrator account' : 'Create your organization'
}

function stepDescription(requiresOrganization: boolean, step: 1 | 2) {
  if (!requiresOrganization) return 'Create an administrator account to get started.'
  return step === 2 ? 'You will own this organization as its first admin.' : undefined
}
