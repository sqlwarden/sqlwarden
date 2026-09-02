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
import { useBrand } from '#/lib/brand/brand'
import { Sidebar, SidebarContent, SidebarInset, SidebarProvider } from '#/components/ui/sidebar'
import { NavigateToLogin } from '#/components/auth/NavigateToLogin'
import { useDesktopRuntime } from '#/lib/desktop/context'

export const Route = createFileRoute('/administration')({
  component: AdministrationLayout,
})

const homeItems: AppShellNavItem[] = [
  { to: '/', label: 'Org Picker', icon: 'arrow-left-01' },
  { to: '/administration', label: 'Overview', icon: 'home-04' },
]

const adminItems: AppShellNavItem[] = [
  { to: '/administration/users', label: 'Users', icon: 'user-multiple-02' },
  { to: '/administration/administrators', label: 'Administrators', icon: 'shield-user' },
  { to: '/administration/organizations', label: 'Organizations', icon: 'building-04' },
  { to: '/administration/instance', label: 'Settings', icon: 'settings-02' },
]

function AdministrationLayout() {
  const brand = useBrand()
  const setupStatus = useSetupStatus()
  const hasToken = Boolean(getAccessToken())
  const session = useSession(hasToken)
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const { preferences, setPreferences } = useAppShellPreferences()
  const [initialOpen] = useState(() => {
    const cookie = document.cookie.split('; ').find((row) => row.startsWith('sidebar_state='))
    return cookie ? cookie.split('=')[1] === 'true' : true
  })
  const desktop = useDesktopRuntime()

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
    return <NavigateToLogin />
  }

  if (setupStatus.data?.capabilities.native_shell) {
    return desktop.session?.identity.org_slug ? <Navigate to="/desktop/settings" replace /> : null
  }

  if (!session.data.is_instance_admin) {
    return <Navigate to="/" replace />
  }

  return (
    <SidebarProvider
      defaultOpen={initialOpen}
      defaultWidth={240}
      style={
        {
          '--sidebar-width-icon': '3rem',
        } as React.CSSProperties
      }
    >
      <Sidebar collapsible="icon" variant={preferences.sidebarStyle}>
        <AppShellHeader label="Administration" icon={<brand.LogoMark size={18} />} />
        <SidebarContent>
          <AppShellNavSection items={homeItems} pathname={pathname} />
          <AppShellNavSection label="Instance" items={adminItems} pathname={pathname} />
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
