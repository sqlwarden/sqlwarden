import type { FormEvent } from 'react'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { PasswordInput } from '#/components/ui/password-input'
import { Icon } from '#/lib/icons'
import { AuthField } from './AuthField'
import { AuthProviderList } from './AuthProviderList'

export function LoginForm({
  email,
  password,
  emailError,
  passwordError,
  isPending,
  redirect,
  onEmailChange,
  onPasswordChange,
  onSubmit,
}: {
  email: string
  password: string
  emailError?: string
  passwordError?: string
  isPending: boolean
  redirect?: string
  onEmailChange: (value: string) => void
  onPasswordChange: (value: string) => void
  onSubmit: (event: FormEvent<HTMLFormElement>) => void
}) {
  return (
    <div className="space-y-6">
      <form className="space-y-4" onSubmit={onSubmit}>
        <AuthField label="Email address" error={emailError}>
          <Input
            autoComplete="email"
            type="email"
            className="h-11 rounded-lg text-sm font-medium"
            value={email}
            onChange={(event) => onEmailChange(event.target.value)}
          />
        </AuthField>

        <AuthField label="Password" error={passwordError}>
          <PasswordInput
            autoComplete="current-password"
            className="h-11 rounded-lg text-sm font-medium"
            value={password}
            onChange={(event) => onPasswordChange(event.target.value)}
          />
        </AuthField>

        <Button
          className="h-11 w-full rounded-lg text-sm shadow-md transition-all duration-150 ease-out hover:shadow-lg active:translate-y-px active:scale-[0.98] active:shadow-sm active:duration-75"
          size="lg"
          disabled={isPending}
          type="submit"
        >
          {isPending ? (
            <span className="flex items-center gap-2">
              <Icon name="loading-03" size={14} className="animate-spin" />
              Signing in…
            </span>
          ) : (
            'Sign in'
          )}
        </Button>
      </form>

      <AuthProviderList redirect={redirect} />
    </div>
  )
}
