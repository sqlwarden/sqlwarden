import { useState } from 'react'
import { Link, Navigate, Outlet, createFileRoute, useRouterState } from '@tanstack/react-router'
import { useLayoutWidth } from '#/components/layout-width-provider'
import { useSession } from '#/hooks/use-session'
import { useSetupStatus } from '#/hooks/use-setup-status'
import { getAccessToken } from '#/lib/auth/access-token'
import { cn } from '#/lib/utils'
import { Separator } from '#/components/ui/separator'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '#/components/ui/breadcrumb'
import {
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
  useSidebar,
} from '#/components/ui/sidebar'
import { HugeiconsIcon } from '@hugeicons/react'
import { Briefcase01Icon, Building04Icon, Key01Icon, ShieldUserIcon, User02Icon } from '@hugeicons/core-free-icons'

export const Route = createFileRoute('/settings')({
  component: SettingsLayout,
})

type NavItem = { to: string; label: string; icon: typeof User02Icon }

const accountItems: NavItem[] = [
  { to: '/settings/account', label: 'Account', icon: User02Icon },
  { to: '/settings/my-organizations', label: 'My Organizations', icon: Briefcase01Icon },
  { to: '/settings/api-tokens', label: 'API Tokens', icon: Key01Icon },
]

const adminItems: NavItem[] = [
  { to: '/settings/administrators', label: 'Administrators', icon: ShieldUserIcon },
  { to: '/settings/organizations', label: 'Organizations', icon: Building04Icon },
]

const allItems = [...accountItems, ...adminItems]

function currentPageLabel(pathname: string): string {
  return allItems.find((item) => item.to === pathname)?.label ?? 'Settings'
}

function SettingsLayout() {
  const setupStatus = useSetupStatus()
  const hasToken = Boolean(getAccessToken())
  const session = useSession(hasToken)
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const { isExpanded } = useLayoutWidth()
  const [initialOpen] = useState(() => {
    const cookie = document.cookie.split('; ').find((row) => row.startsWith('sidebar_state='))
    return cookie ? cookie.split('=')[1] === 'true' : true
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

  return (
    <SidebarProvider
      defaultOpen={initialOpen}
      style={{
        '--sidebar-width': '15rem',
        '--sidebar-width-icon': '3rem',
        minHeight: 'calc(100vh - 4rem)',
      } as React.CSSProperties}
    >
      <SettingsSidebar session={session.data} pathname={pathname} />
      <main className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-12 shrink-0 items-center gap-2 border-b px-4">
          <SidebarTrigger />
          <Breadcrumb>
            <BreadcrumbList>
              <BreadcrumbItem>
                <BreadcrumbLink render={<Link to="/settings/account" />}>Settings</BreadcrumbLink>
              </BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem>
                <BreadcrumbPage>{currentPageLabel(pathname)}</BreadcrumbPage>
              </BreadcrumbItem>
            </BreadcrumbList>
          </Breadcrumb>
        </header>
        <div
          className={cn(
            'flex-1 py-8',
            isExpanded ? 'px-4 sm:px-6' : 'container mx-auto px-4',
          )}
        >
          <Outlet />
        </div>
      </main>
    </SidebarProvider>
  )
}

function SettingsSidebar({
  session,
  pathname,
}: {
  session: { is_instance_admin: boolean }
  pathname: string
}) {
  const { state } = useSidebar()
  const isCollapsed = state === 'collapsed'

  return (
    <div
      className="group flex shrink-0 flex-col border-r text-sidebar-foreground transition-[width] duration-200 ease-linear"
      data-state={state}
      data-collapsible={isCollapsed ? 'icon' : ''}
      data-side="left"
      style={{ width: isCollapsed ? 'var(--sidebar-width-icon)' : 'var(--sidebar-width)' }}
    >
      <div data-sidebar="sidebar" className="flex size-full flex-col bg-sidebar">
        <SidebarHeader className={cn('h-12 justify-center', isCollapsed ? 'items-center' : 'px-4')}>
          {isCollapsed ? null : (
            <p className="text-sm font-semibold">Settings</p>
          )}
        </SidebarHeader>

        <SidebarContent className="pt-2">
          <SidebarGroup>
            <SidebarGroupLabel>Account</SidebarGroupLabel>
            <SidebarMenu>
              {accountItems.map((item) => (
                <SidebarMenuItem key={item.to}>
                  <SidebarMenuButton
                    render={<Link to={item.to} />}
                    isActive={pathname === item.to}
                    tooltip={item.label}
                  >
                    <HugeiconsIcon icon={item.icon} strokeWidth={2} />
                    <span>{item.label}</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              ))}
            </SidebarMenu>
          </SidebarGroup>

          {session.is_instance_admin ? (
            <SidebarGroup>
              <SidebarGroupLabel>Instance Admin</SidebarGroupLabel>
              <SidebarMenu>
                {adminItems.map((item) => (
                  <SidebarMenuItem key={item.to}>
                    <SidebarMenuButton
                      render={<Link to={item.to} />}
                      isActive={pathname === item.to}
                      tooltip={item.label}
                    >
                      <HugeiconsIcon icon={item.icon} strokeWidth={2} />
                      <span>{item.label}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroup>
          ) : null}
        </SidebarContent>
      </div>
    </div>
  )
}
