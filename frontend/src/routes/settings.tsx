import { useState } from 'react'
import { Link, Navigate, Outlet, createFileRoute, useRouterState } from '@tanstack/react-router'
import { Briefcase, Building2, KeyRound, PanelLeftClose, PanelLeftOpen, ShieldCheck, User, type LucideIcon } from 'lucide-react'
import { useLayoutWidth } from '#/components/layout-width-provider'
import { useSession } from '#/hooks/use-session'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { getAccessToken } from '#/lib/auth/access-token'
import { cn } from '#/lib/utils'
import { Button } from '#/components/ui/button'
import { Separator } from '#/components/ui/separator'
import { Tooltip, TooltipContent, TooltipTrigger } from '#/components/ui/tooltip'

export const Route = createFileRoute('/settings')({
  component: SettingsLayout,
})

const accountItems = [
  { to: '/settings/account', label: 'Account', icon: User },
  { to: '/settings/my-organizations', label: 'My Organizations', icon: Briefcase },
  { to: '/settings/api-tokens', label: 'API Tokens', icon: KeyRound },
] as const

const adminItems = [
  { to: '/settings/administrators', label: 'Administrators', icon: ShieldCheck },
  { to: '/settings/organizations', label: 'Organizations', icon: Building2 },
] as const

function SettingsLayout() {
  const setupStatus = useSetupStatus()
  const hasToken = Boolean(getAccessToken())
  const session = useSession(hasToken)
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const { isExpanded } = useLayoutWidth()
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(() => {
    const stored = window.localStorage.getItem('sqlwarden.settings_sidebar_collapsed')
    return stored === 'true'
  })

  if (setupStatus.isLoading || (hasToken && session.isLoading)) {
    return (
      <main className="flex min-h-[calc(100vh-4rem)] items-center justify-center px-4">
        <div className="text-sm text-muted-foreground">Loading…</div>
      </main>
    )
  }

  if (setupStatus.data && !setupStatus.data.configured) {
    return <Navigate to="/setup" replace />
  }

  if (!hasToken || !session.data) {
    return <Navigate to="/login" replace />
  }

  function toggleSidebarCollapsed() {
    setIsSidebarCollapsed((current) => {
      const next = !current
      window.localStorage.setItem('sqlwarden.settings_sidebar_collapsed', String(next))
      return next
    })
  }

  function renderNavItem(item: { to: string; label: string; icon: LucideIcon }) {
    const Icon = item.icon
    const isActive = pathname === item.to

    const link = (
      <Link
        key={item.to}
        to={item.to}
        className={cn(
          'relative flex items-center rounded-md py-2 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground',
          isSidebarCollapsed ? 'justify-center px-1' : 'gap-2 px-3',
          isActive && isSidebarCollapsed ? 'bg-muted text-foreground' : null,
          isActive && !isSidebarCollapsed ? 'bg-muted text-foreground' : null,
        )}
      >
        {isActive ? (
          <span
            className={cn(
              'absolute top-1/2 h-4 w-0.5 -translate-y-1/2 rounded-full bg-primary',
              isSidebarCollapsed ? '-start-1' : '-start-1.5',
            )}
          />
        ) : null}
        <Icon className="size-4" />
        {isSidebarCollapsed ? null : item.label}
      </Link>
    )

    if (!isSidebarCollapsed) {
      return link
    }

    return (
      <Tooltip key={item.to}>
        <TooltipTrigger render={link} />
        <TooltipContent side="right">{item.label}</TooltipContent>
      </Tooltip>
    )
  }

  return (
    <main
      className={cn(
        'flex min-h-[calc(100vh-4rem)] py-8',
        isExpanded ? 'w-full px-4 sm:px-6' : 'container mx-auto px-4',
      )}
    >
      <div
        className={cn(
          'grid w-full gap-8',
          isSidebarCollapsed ? 'md:grid-cols-[44px_auto_minmax(0,1fr)]' : 'md:grid-cols-[240px_auto_minmax(0,1fr)]',
        )}
      >
        <aside className="flex flex-col gap-4">
          <div className={cn('flex items-start pb-2', isSidebarCollapsed ? 'justify-center' : 'justify-between px-3')}>
            {isSidebarCollapsed ? null : (
              <div>
                <h1 className="text-lg font-semibold tracking-tight">Settings</h1>
                <p className="text-sm text-muted-foreground">Manage your account and instance configuration.</p>
              </div>
            )}
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="cursor-pointer"
              onClick={toggleSidebarCollapsed}
              aria-label={isSidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
              title={isSidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
            >
              {isSidebarCollapsed ? <PanelLeftOpen /> : <PanelLeftClose />}
            </Button>
          </div>

          <section className="flex flex-col gap-2">
            {isSidebarCollapsed ? null : (
              <div className="flex items-center gap-3 px-3">
                <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Account</span>
                <Separator className="flex-1" />
              </div>
            )}
            <nav className="flex flex-col gap-1">
              {accountItems.map(renderNavItem)}
            </nav>
          </section>

          {session.data.is_instance_admin ? (
            <section className="flex flex-col gap-2">
              {isSidebarCollapsed ? null : (
                <div className="flex items-center gap-3 px-3">
                  <span className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Instance Admin</span>
                  <Separator className="flex-1" />
                </div>
              )}
              <nav className="flex flex-col gap-1">
                {adminItems.map(renderNavItem)}
              </nav>
            </section>
          ) : null}
        </aside>

        <Separator orientation="vertical" className="hidden h-full md:block" />

        <section className="min-w-0">
          <Outlet />
        </section>
      </div>
    </main>
  )
}
