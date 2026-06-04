import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
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
import { orgEffectivePermissionsQueryOptions, orgQueryOptions, orgWorkspaceQueryOptions } from '#/lib/api/query'
import { getAccessToken } from '#/lib/auth/access-token'
import { hasAnyPermission, permission, type Permission } from '#/lib/permissions'
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
  const workspaceId = workspaceIdFromPath(pathname, orgSlug)
  const organization = useQuery({
    ...orgQueryOptions(orgSlug),
    enabled: hasToken,
  })
  const workspace = useQuery({
    ...orgWorkspaceQueryOptions(orgSlug, workspaceId ?? ''),
    enabled: Boolean(workspaceId && hasToken),
  })
  const orgEffectivePermissions = useQuery({
    ...orgEffectivePermissionsQueryOptions(orgSlug, 'org'),
    enabled: Boolean(!workspaceId && hasToken),
  })
  const workspaceEffectivePermissions = useQuery({
    ...orgEffectivePermissionsQueryOptions(orgSlug, 'workspace', workspaceId ?? ''),
    enabled: Boolean(workspaceId && hasToken),
  })
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

  const workspacePermissions = workspaceEffectivePermissions.data?.permissions
  const workspacePrimaryNavItems = workspaceId ? workspacePrimaryItems(orgSlug, workspaceId, workspacePermissions) : []
  const workspaceAccessControlNavItems = workspaceId ? workspaceAccessControlItems(orgSlug, workspaceId, workspacePermissions) : []
  const workspaceSettingsNavItems = workspaceId ? workspaceSettingsItems(orgSlug, workspaceId, workspacePermissions) : []
  const orgAccessControlNavItems = !workspaceId ? accessControlItems(orgSlug, orgEffectivePermissions.data?.permissions) : []
  const orgSettingsNavItems = !workspaceId ? settingsItems(orgSlug, orgEffectivePermissions.data?.permissions) : []

  if (
    workspaceId &&
    workspaceEffectivePermissions.data &&
    !isWorkspacePathAllowed(pathname, orgSlug, workspaceId, workspaceEffectivePermissions.data.permissions)
  ) {
    return (
      <Navigate
        to="/orgs/$org_slug/workspaces/$workspace_id"
        params={{ org_slug: orgSlug, workspace_id: workspaceId }}
        replace
      />
    )
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
        <AppShellHeader
          label="SQLWarden"
          description={
            workspaceId
              ? `${organization.data?.name ?? orgSlug} / ${workspace.data?.name ?? `Workspace #${workspaceId}`}`
              : organization.data?.name ?? orgSlug
          }
          icon="database-lightning"
        />
        <SidebarContent>
          {workspaceId ? (
            <>
              <AppShellNavSection items={workspacePrimaryNavItems} pathname={pathname} />
              {workspaceAccessControlNavItems.length > 0 ? (
                <AppShellNavSection label="Access Control" items={workspaceAccessControlNavItems} pathname={pathname} />
              ) : null}
              {workspaceSettingsNavItems.length > 0 ? (
                <AppShellNavSection items={workspaceSettingsNavItems} pathname={pathname} />
              ) : null}
            </>
          ) : (
            <>
              <AppShellNavSection items={organizationItems(orgSlug)} pathname={pathname} />
              {orgAccessControlNavItems.length > 0 ? (
                <AppShellNavSection label="Access Control" items={orgAccessControlNavItems} pathname={pathname} />
              ) : null}
              {orgSettingsNavItems.length > 0 ? (
                <AppShellNavSection label="Settings" items={orgSettingsNavItems} pathname={pathname} />
              ) : null}
            </>
          )}
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

const workspaceEnvironmentPagePermissions = [
  permission.envRead,
  permission.envWrite,
  permission.envCreate,
  permission.envDelete,
  permission.envDeploy,
  permission.connRead,
  permission.connWrite,
  permission.connCreate,
  permission.connDelete,
  permission.connExecute,
  permission.connDql,
  permission.connDml,
  permission.connDdl,
] as const satisfies readonly Permission[]

const workspaceConnectionPagePermissions = [
  permission.connRead,
  permission.connWrite,
  permission.connCreate,
  permission.connDelete,
  permission.connExecute,
  permission.connDql,
  permission.connDml,
  permission.connDdl,
] as const satisfies readonly Permission[]

const workspacePolicyPagePermissions = [
  permission.policyRead,
  permission.policyModify,
] as const satisfies readonly Permission[]

const workspaceSettingsPagePermissions = [
  permission.wsRead,
  permission.wsWrite,
] as const satisfies readonly Permission[]

function workspacePrimaryItems(orgSlug: string, workspaceId: string, permissions: readonly string[] | undefined): AppShellNavItem[] {
  const items: AppShellNavItem[] = [
    { to: '/orgs/$org_slug/workspaces', params: { org_slug: orgSlug }, label: 'All Workspaces', icon: 'arrow-left-01' },
    { to: '/orgs/$org_slug/workspaces/$workspace_id', params: { org_slug: orgSlug, workspace_id: workspaceId }, label: 'Overview', icon: 'home-04' },
  ]

  if (hasAnyPermission(permissions, workspaceEnvironmentPagePermissions)) {
    items.push({ to: '/orgs/$org_slug/workspaces/$workspace_id/environments', params: { org_slug: orgSlug, workspace_id: workspaceId }, label: 'Environments', icon: 'database' })
  }
  if (hasAnyPermission(permissions, workspaceConnectionPagePermissions)) {
    items.push({ to: '/orgs/$org_slug/workspaces/$workspace_id/connections', params: { org_slug: orgSlug, workspace_id: workspaceId }, label: 'Connections', icon: 'flow-connection' })
  }

  return items
}

function workspaceAccessControlItems(orgSlug: string, workspaceId: string, permissions: readonly string[] | undefined): AppShellNavItem[] {
  if (!hasAnyPermission(permissions, workspacePolicyPagePermissions)) {
    return []
  }

  return [
    { to: '/orgs/$org_slug/workspaces/$workspace_id/users', params: { org_slug: orgSlug, workspace_id: workspaceId }, label: 'Users', icon: 'user-multiple' },
    { to: '/orgs/$org_slug/workspaces/$workspace_id/teams', params: { org_slug: orgSlug, workspace_id: workspaceId }, label: 'Teams', icon: 'user-group' },
    { to: '/orgs/$org_slug/workspaces/$workspace_id/roles', params: { org_slug: orgSlug, workspace_id: workspaceId }, label: 'Roles', icon: 'user-shield-01' },
    { to: '/orgs/$org_slug/workspaces/$workspace_id/policies', params: { org_slug: orgSlug, workspace_id: workspaceId }, label: 'Policies', icon: 'policy' },
  ]
}

function workspaceSettingsItems(orgSlug: string, workspaceId: string, permissions: readonly string[] | undefined): AppShellNavItem[] {
  if (!hasAnyPermission(permissions, workspaceSettingsPagePermissions)) {
    return []
  }

  return [{ to: '/orgs/$org_slug/workspaces/$workspace_id/settings', params: { org_slug: orgSlug, workspace_id: workspaceId }, label: 'Settings', icon: 'settings-02' }]
}

function isWorkspacePathAllowed(pathname: string, orgSlug: string, workspaceId: string, permissions: readonly string[]) {
  const basePath = `/orgs/${orgSlug}/workspaces/${workspaceId}`
  const path = trimTrailingSlash(pathname)

  if (path === basePath) {
    return true
  }
  if (isPathInSection(path, basePath, 'environments')) {
    return hasAnyPermission(permissions, workspaceEnvironmentPagePermissions)
  }
  if (isPathInSection(path, basePath, 'connections')) {
    return hasAnyPermission(permissions, workspaceConnectionPagePermissions)
  }
  if (isPathInSection(path, basePath, 'users') || isPathInSection(path, basePath, 'teams') || isPathInSection(path, basePath, 'roles') || isPathInSection(path, basePath, 'policies')) {
    return hasAnyPermission(permissions, workspacePolicyPagePermissions)
  }
  if (isPathInSection(path, basePath, 'settings')) {
    return hasAnyPermission(permissions, workspaceSettingsPagePermissions)
  }

  return true
}

function isPathInSection(pathname: string, basePath: string, section: string) {
  const sectionPath = `${basePath}/${section}`
  return pathname === sectionPath || pathname.startsWith(`${sectionPath}/`)
}

function organizationItems(orgSlug: string): AppShellNavItem[] {
  return [
    { to: '/orgs/$org_slug/workspaces', params: { org_slug: orgSlug }, label: 'Workspaces', icon: 'briefcase-01' },
    { to: '/orgs/$org_slug/ide', params: { org_slug: orgSlug }, label: 'IDE', icon: 'terminal' },
  ]
}

function accessControlItems(orgSlug: string, permissions: readonly string[] | undefined): AppShellNavItem[] {
  const items: AppShellNavItem[] = []

  if (hasAnyPermission(permissions, [permission.orgRead])) {
    items.push(
      { to: '/orgs/$org_slug/users', params: { org_slug: orgSlug }, label: 'Users', icon: 'user-multiple' },
      { to: '/orgs/$org_slug/teams', params: { org_slug: orgSlug }, label: 'Teams', icon: 'user-group' },
    )
  }

  if (hasAnyPermission(permissions, [permission.policyRead])) {
    items.push(
      { to: '/orgs/$org_slug/roles', params: { org_slug: orgSlug }, label: 'Roles', icon: 'user-shield-01' },
      { to: '/orgs/$org_slug/policies', params: { org_slug: orgSlug }, label: 'Policies', icon: 'user-lock-02' },
    )
  }

  return items
}

function settingsItems(orgSlug: string, permissions: readonly string[] | undefined): AppShellNavItem[] {
  if (!hasAnyPermission(permissions, [permission.orgRead, permission.orgWrite, permission.orgDelete])) {
    return []
  }

  return [
    { to: '/orgs/$org_slug/settings/general', params: { org_slug: orgSlug }, label: 'General', icon: 'settings-02' },
  ]
}

function workspaceIdFromPath(pathname: string, orgSlug: string) {
  const pattern = new RegExp(`^/orgs/${escapeRegExp(orgSlug)}/workspaces/([^/]+)(?:/|$)`)
  const match = pathname.match(pattern)
  return match?.[1]
}

function trimTrailingSlash(path: string) {
  return path === '/' ? path : path.replace(/\/$/, '')
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}
