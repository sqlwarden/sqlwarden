import { useState } from 'react'
import { Navigate, Outlet, createFileRoute, useRouterState } from '@tanstack/react-router'
import {
  AppShellContent,
  AppShellHeader,
  AppShellNavSection,
  AppShellRail,
  AppShellSidebarFooter,
  useAppShellPreferences,
  type AppShellNavItem,
} from '#/components/app-shell'
import { useSession } from '#/hooks/use-session'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { getAccessToken } from '#/lib/auth/access-token'
import { Sidebar, SidebarContent, SidebarInset, SidebarProvider } from '#/components/ui/sidebar'

export const Route = createFileRoute('/settings')({
  component: SettingsLayout,
})

const accountItems: AppShellNavItem[] = [
  { to: '/settings/account', label: 'Account', icon: 'user-02' },
  { to: '/settings/my-organizations', label: 'My Organizations', icon: 'briefcase-01' },
  { to: '/settings/api-tokens', label: 'API Tokens', icon: 'key-01' },
]

const adminItems: AppShellNavItem[] = [
  { to: '/settings/instance', label: 'Instance', icon: 'settings-02' },
  { to: '/settings/users', label: 'Users', icon: 'user-multiple-02' },
  { to: '/settings/administrators', label: 'Administrators', icon: 'shield-user' },
  { to: '/settings/organizations', label: 'Organizations', icon: 'building-04' },
]

function SettingsLayout() {
  const setupStatus = useSetupStatus()
  const hasToken = Boolean(getAccessToken())
  const session = useSession(hasToken)
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const { preferences, setPreferences } = useAppShellPreferences()
  const [initialOpen] = useState(() => {
    const cookie = document.cookie.split('; ').find((row) => row.startsWith('sidebar_state='))
    return cookie ? cookie.split('=')[1] === 'true' : true
  })

  if (setupStatus.isLoading || (hasToken && session.isLoading)) {
    return (
      <main className="flex min-h-screen items-center justify-center px-4">
        <div className="text-sm text-muted-foreground">Loading...</div>
      </main>
    )
  }

  if (setupStatus.data && !setupStatus.data.configured) {
    return <Navigate to="/setup" replace />
  }

  if (!hasToken || !session.data) {
    return <Navigate to="/login" replace />
  }

  return (
    <SidebarProvider
      defaultOpen={initialOpen}
      defaultWidth={240}
      style={{
        '--sidebar-width-icon': '3rem',
      } as React.CSSProperties}
    >
      <Sidebar collapsible="icon" variant={preferences.sidebarStyle}>
        <AppShellHeader label="Settings" icon="settings-02" />
        <SidebarContent>
          <AppShellNavSection label="Account" items={accountItems} pathname={pathname} />
          {session.data.is_instance_admin ? (
            <AppShellNavSection label="Instance Admin" items={adminItems} pathname={pathname} />
          ) : null}
        </SidebarContent>
        <AppShellSidebarFooter
          session={session.data}
          preferences={preferences}
          setPreferences={setPreferences}
        />
        <AppShellRail />
      </Sidebar>
      <SidebarInset className="min-w-0 bg-background">
        <AppShellContent preferences={preferences}>
          <Outlet />
        </AppShellContent>
      </SidebarInset>
    </SidebarProvider>
  )
}
