import type { ReactNode } from 'react'
import { useBrand } from '#/lib/brand/brand'
import { cn } from '#/lib/utils'
import './login-surface.css'
import { ThemeSelector } from './ThemeSelector'

export function LoginSurface({
  title,
  description,
  children,
  footer,
  eyebrow,
  className,
}: {
  title: string
  description: string
  children: ReactNode
  footer?: ReactNode
  eyebrow?: ReactNode
  className?: string
}) {
  const brand = useBrand()

  return (
    <div className="relative flex min-h-svh flex-col items-center justify-center px-4 py-12">
      <div className="absolute top-4 right-4 sm:top-6 sm:right-6">
        <ThemeSelector />
      </div>

      <brand.LogoColorLockup size={28} className="mb-7 text-foreground" />

      <div
        className={cn(
          'auth-card relative w-full max-w-[420px] rounded-[18px] border border-border/70 bg-card p-8 text-card-foreground',
          'motion-safe:animate-in motion-safe:fade-in-0 motion-safe:slide-in-from-bottom-4 motion-safe:duration-[420ms] motion-safe:ease-out',
          className,
        )}
      >
        <div className="space-y-1.5 text-center">
          {eyebrow}
          <h1 className="font-heading text-xl font-semibold tracking-tight">{title}</h1>
          <p className="text-sm text-muted-foreground">{description}</p>
        </div>

        <div className="mt-7">{children}</div>

        {footer ? <div className="mt-6">{footer}</div> : null}
      </div>
    </div>
  )
}
