import type { FormEvent } from 'react'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { PasswordInput } from '#/components/ui/password-input'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
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
  const hasError = Boolean(emailError || passwordError)

  return (
    <div className="space-y-6">
      <form className="space-y-5" onSubmit={onSubmit}>
        <div className="space-y-1.5">
          <div
            className={cn(
              'divide-y divide-input overflow-hidden rounded-xl border border-input bg-input/20 shadow-sm transition-colors focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/30 dark:bg-input/30',
              hasError && 'border-destructive/60 focus-within:ring-destructive/20',
            )}
          >
            <Input
              autoComplete="email"
              type="email"
              placeholder="Email address"
              aria-label="Email address"
              aria-invalid={Boolean(emailError)}
              className="h-11 rounded-none border-0 bg-transparent px-3.5 text-sm font-medium shadow-none focus-visible:border-0 focus-visible:ring-0"
              value={email}
              onChange={(event) => onEmailChange(event.target.value)}
            />
            <PasswordInput
              autoComplete="current-password"
              placeholder="Password"
              aria-label="Password"
              aria-invalid={Boolean(passwordError)}
              className="h-11 rounded-none border-0 bg-transparent pl-3.5 text-sm font-medium shadow-none focus-visible:border-0 focus-visible:ring-0"
              value={password}
              onChange={(event) => onPasswordChange(event.target.value)}
            />
          </div>
          {hasError ? (
            <div className="space-y-1">
              {emailError ? <p className="text-xs text-destructive">{emailError}</p> : null}
              {passwordError ? <p className="text-xs text-destructive">{passwordError}</p> : null}
            </div>
          ) : null}
        </div>

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
