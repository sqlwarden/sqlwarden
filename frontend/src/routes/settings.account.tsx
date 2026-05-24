import { useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { toast } from 'sonner'
import { useSession } from '#/hooks/use-session'
import { api } from '#/lib/api/client'
import { isApiError } from '#/lib/api/errors'
import { queryKeys } from '#/lib/api/query'
import type { Account } from '#/lib/api/types'
import { Button } from '#/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '#/components/ui/card'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { RoutePending } from '#/components/RoutePending'

export const Route = createFileRoute('/settings/account')({
  component: SettingsAccountPage,
  pendingComponent: RoutePending,
})

type ProfileErrors = {
  name?: string
}

type PasswordErrors = {
  current_password?: string
  new_password?: string
  confirm_password?: string
}

function SettingsAccountPage() {
  const queryClient = useQueryClient()
  const session = useSession(true)
  const account = session.data?.account
  const [name, setName] = useState('')
  const [profileErrors, setProfileErrors] = useState<ProfileErrors>({})
  const [passwordValues, setPasswordValues] = useState({
    currentPassword: '',
    newPassword: '',
    confirmPassword: '',
  })
  const [passwordErrors, setPasswordErrors] = useState<PasswordErrors>({})

  useEffect(() => {
    if (account) {
      setName(account.name)
    }
  }, [account])

  const updateProfile = useMutation({
    mutationFn: async () => api.patch<Account>('/api/v1/account', { name: name.trim() }),
    onSuccess: async () => {
      setProfileErrors({})
      toast.success('Profile updated')
      await queryClient.invalidateQueries({ queryKey: queryKeys.session() })
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors?.name) {
        setProfileErrors({ name: error.fieldErrors.name })
        return
      }
      toast.error(error instanceof Error ? error.message : 'Failed to update profile')
    },
  })

  const updatePassword = useMutation({
    mutationFn: async () =>
      api.patch<void>('/api/v1/account/password', {
        current_password: passwordValues.currentPassword,
        new_password: passwordValues.newPassword,
      }),
    onSuccess: () => {
      setPasswordValues({
        currentPassword: '',
        newPassword: '',
        confirmPassword: '',
      })
      setPasswordErrors({})
      toast.success('Password changed')
    },
    onError: (error) => {
      if (isApiError(error) && error.fieldErrors) {
        const fieldErrors: PasswordErrors = {
          current_password: error.fieldErrors.current_password,
          new_password: error.fieldErrors.new_password,
        }
        if (fieldErrors.current_password || fieldErrors.new_password) {
          setPasswordErrors(fieldErrors)
          return
        }
      }
      toast.error(error instanceof Error ? error.message : 'Failed to change password')
    },
  })

  if (!account) {
    return <RoutePending />
  }

  function submitProfile(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()

    if (!name.trim()) {
      setProfileErrors({ name: 'Name is required.' })
      return
    }

    setProfileErrors({})
    void updateProfile.mutateAsync().catch(() => {})
  }

  function updatePasswordField(field: keyof typeof passwordValues, value: string) {
    setPasswordValues((current) => ({ ...current, [field]: value }))
    setPasswordErrors((current) => {
      const next = { ...current }
      if (field === 'currentPassword') delete next.current_password
      if (field === 'newPassword') delete next.new_password
      if (field === 'confirmPassword') delete next.confirm_password
      return next
    })
  }

  function submitPassword(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()

    const nextErrors: PasswordErrors = {}
    if (!passwordValues.currentPassword) nextErrors.current_password = 'Current password is required.'
    if (!passwordValues.newPassword) nextErrors.new_password = 'New password is required.'
    else if (passwordValues.newPassword.length < 8) nextErrors.new_password = 'New password must be at least 8 characters.'
    if (!passwordValues.confirmPassword) nextErrors.confirm_password = 'Confirm the new password.'
    else if (passwordValues.newPassword !== passwordValues.confirmPassword) nextErrors.confirm_password = 'Passwords do not match.'

    if (Object.keys(nextErrors).length > 0) {
      setPasswordErrors(nextErrors)
      return
    }

    setPasswordErrors({})
    void updatePassword.mutateAsync().catch(() => {})
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-1.5">
        <h2 className="text-2xl font-semibold tracking-tight">Account</h2>
        <p className="text-sm text-muted-foreground">Manage your profile and local password.</p>
      </div>

      <Card>
        <CardHeader className="border-b border-border">
          <CardTitle>Profile</CardTitle>
          <CardDescription>Your profile is visible to administrators and organization members where you have access.</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-5" onSubmit={submitProfile}>
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="Full name" error={profileErrors.name}>
                <Input
                  value={name}
                  autoComplete="name"
                  disabled={updateProfile.isPending}
                  onChange={(event) => {
                    setName(event.target.value)
                    setProfileErrors({})
                  }}
                />
              </Field>
              <Field label="Email address">
                <Input value={account.email} type="email" disabled />
              </Field>
            </div>
            <div className="flex justify-end">
              <Button type="submit" disabled={updateProfile.isPending || name.trim() === account.name}>
                {updateProfile.isPending ? 'Saving...' : 'Save profile'}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="border-b border-border">
          <CardTitle>Password</CardTitle>
          <CardDescription>Change the password used for local SQLWarden sign-in.</CardDescription>
        </CardHeader>
        <CardContent>
          <form className="flex flex-col gap-5" onSubmit={submitPassword}>
            <Field label="Current password" error={passwordErrors.current_password}>
              <Input
                type="password"
                autoComplete="current-password"
                value={passwordValues.currentPassword}
                disabled={updatePassword.isPending}
                onChange={(event) => updatePasswordField('currentPassword', event.target.value)}
              />
            </Field>

            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="New password" error={passwordErrors.new_password}>
                <Input
                  type="password"
                  autoComplete="new-password"
                  value={passwordValues.newPassword}
                  disabled={updatePassword.isPending}
                  onChange={(event) => updatePasswordField('newPassword', event.target.value)}
                />
              </Field>
              <Field label="Confirm new password" error={passwordErrors.confirm_password}>
                <Input
                  type="password"
                  autoComplete="new-password"
                  value={passwordValues.confirmPassword}
                  disabled={updatePassword.isPending}
                  onChange={(event) => updatePasswordField('confirmPassword', event.target.value)}
                />
              </Field>
            </div>

            <div className="flex justify-end">
              <Button type="submit" disabled={updatePassword.isPending}>
                {updatePassword.isPending ? 'Changing...' : 'Change password'}
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
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
    <div className="flex flex-col gap-2">
      <Label>{label}</Label>
      {children}
      {error ? <p className="text-xs text-destructive">{error}</p> : null}
    </div>
  )
}
