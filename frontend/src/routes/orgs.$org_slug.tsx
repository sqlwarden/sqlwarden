import { useState } from 'react'
import { Navigate, Outlet, createFileRoute, useRouterState } from '@tanstack/react-router'
import {
  Briefcase01Icon,
  Building04Icon,
  Home04Icon,
  Settings02Icon,
  UserGroupIcon,
  UserLock02Icon,
  UserMultipleIcon,
  UserShield01Icon,
  TerminalIcon,
} from '@hugeicons/core-free-icons'
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

export const Route = createFileRoute('/orgs/$org_slug')({
  component: OrganizationLayout,
})

function OrganizationLayout() {
  const setupStatus = useSetupStatus()
  const hasToken = Boolean(getAccessToken())
  const session = useSession(hasToken)
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const { org_slug: orgSlug } = Route.useParams()
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
      style={{
        '--sidebar-width': '15rem',
        '--sidebar-width-icon': '3rem',
      } as React.CSSProperties}
    >
      <Sidebar collapsible="icon" variant={preferences.sidebarStyle}>
        <AppShellHeader label="SQLWarden" description={`@${orgSlug}`} icon={Building04Icon} />
        <SidebarContent>
          <AppShellNavSection items={organizationItems(orgSlug)} pathname={pathname} />
          <AppShellNavSection label="Access Control" items={accessControlItems(orgSlug)} pathname={pathname} />
          <AppShellNavSection items={settingsItems(orgSlug)} pathname={pathname} />
        </SidebarContent>
        <AppShellSidebarFooter
          session={session.data}
          preferences={preferences}
          setPreferences={setPreferences}
          extraUserItems={[
            { to: '/settings/my-organizations', label: 'Switch Organization', icon: Building04Icon },
          ]}
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

function organizationItems(orgSlug: string): AppShellNavItem[] {
  return [
    { to: '/orgs/$org_slug', params: { org_slug: orgSlug }, label: 'Home', icon: Home04Icon },
    { to: '/orgs/$org_slug/workspaces', params: { org_slug: orgSlug }, label: 'Workspaces', icon: Briefcase01Icon },
    { to: '/orgs/$org_slug/ide', params: { org_slug: orgSlug }, label: 'IDE', icon: TerminalIcon },
  ]
}

function accessControlItems(orgSlug: string): AppShellNavItem[] {
  return [
    { to: '/orgs/$org_slug/users', params: { org_slug: orgSlug }, label: 'Users', icon: UserMultipleIcon },
    { to: '/orgs/$org_slug/teams', params: { org_slug: orgSlug }, label: 'Teams', icon: UserGroupIcon },
    { to: '/orgs/$org_slug/roles', params: { org_slug: orgSlug }, label: 'Roles', icon: UserShield01Icon },
    { to: '/orgs/$org_slug/policies', params: { org_slug: orgSlug }, label: 'Policies', icon: UserLock02Icon },
  ]
}

function settingsItems(orgSlug: string): AppShellNavItem[] {
  return [
    { to: '/orgs/$org_slug/settings', params: { org_slug: orgSlug }, label: 'Settings', icon: Settings02Icon },
  ]
}
