import type { ReactNode } from 'react'
import { Icon } from '#/lib/icons'
import type { AppIcon } from '#/lib/icons'
import { useBrand } from '#/lib/brand/brand'
import { cn } from '#/lib/utils'

const PANEL_HIGHLIGHTS: Array<{ icon: AppIcon; title: string; description: string }> = [
  {
    icon: 'policy',
    title: 'Custom RBAC',
    description: 'Grant exactly the access each workspace, environment, and connection needs.',
  },
  {
    icon: 'flow-connection',
    title: 'Every environment',
    description: 'PostgreSQL, MySQL, and SQLite connections, side by side, production to local.',
  },
  {
    icon: 'terminal',
    title: 'A real SQL IDE',
    description: 'Explorer, editor, and results in one workspace built for teams.',
  },
]

export function AuthLayout({
  eyebrow,
  title,
  description,
  children,
  footer,
  contentClassName,
}: {
  eyebrow?: ReactNode
  title: string
  description: string
  children: ReactNode
  footer?: ReactNode
  contentClassName?: string
}) {
  const brand = useBrand()

  return (
    <main className="grid min-h-svh lg:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)]">
      <section className="relative hidden flex-col justify-between overflow-hidden bg-neutral-950 px-12 py-12 text-neutral-50 lg:flex">
        <div
          aria-hidden="true"
          className="pointer-events-none absolute -top-24 -right-24 size-[28rem] rounded-full bg-primary/20 blur-3xl"
        />
        <brand.LogoLockup size={22} className="relative" />
        <div className="relative flex flex-col gap-10">
          <h2 className="max-w-sm text-3xl leading-tight font-semibold tracking-tight text-balance">
            The SQL IDE built for controlled database access.
          </h2>
          <ul className="flex flex-col gap-5">
            {PANEL_HIGHLIGHTS.map((item) => (
              <li key={item.title} className="flex items-start gap-3">
                <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-white/10 text-primary">
                  <Icon name={item.icon} size={16} />
                </span>
                <div className="flex flex-col gap-0.5">
                  <p className="text-sm font-medium">{item.title}</p>
                  <p className="text-xs text-neutral-400">{item.description}</p>
                </div>
              </li>
            ))}
          </ul>
        </div>
        <p className="relative text-xs text-neutral-400">
          &copy; {new Date().getFullYear()} {brand.productName}
        </p>
      </section>

      <section className="flex items-center justify-center px-4 py-12 sm:px-8">
        <div className={cn('w-full max-w-sm space-y-8', contentClassName)}>
          <div className="flex flex-col items-center gap-6 lg:hidden">
            <brand.LogoLockup size={20} />
          </div>
          <div className="space-y-2 text-center lg:text-left">
            {eyebrow}
            <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
            <p className="text-sm text-muted-foreground">{description}</p>
          </div>
          {children}
          {footer}
        </div>
      </section>
    </main>
  )
}
