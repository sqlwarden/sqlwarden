import { trimTrailingSlash } from '#/lib/utils'
import { useWorkspacePageTitle } from '#/lib/page-title'
import { useQuery } from '@tanstack/react-query'
import { Link, Outlet, createFileRoute, useRouterState } from '@tanstack/react-router'
import { Icon, type AppIcon } from '#/lib/icons'
import { orgEffectivePermissionsQueryOptions, orgWorkspaceQueryOptions } from '#/lib/api/query'
import { hasAnyPermission, type Permission } from '#/lib/permissions'
import {
  workspaceConnectionPagePermissions,
  workspaceEnvironmentPagePermissions,
  workspacePolicyPagePermissions,
  workspaceSettingsPagePermissions,
} from '#/lib/workspace-page-permissions'
import { Button } from '#/components/ui/button'
import { Skeleton } from '#/components/ui/skeleton'
import { EmptyState } from '#/components/EmptyState'
import { errorMessage, isApiError } from '#/lib/api/errors'
import { useSetupStatus } from '#/hooks/use-setup-status'

export const Route = createFileRoute('/orgs/$org_slug/workspaces/$workspace_id')({
  component: WorkspaceRoute,
})

function WorkspaceRoute() {
  const { org_slug: orgSlug, workspace_id: workspaceId } = Route.useParams()
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const overviewPath = `/orgs/${orgSlug}/workspaces/${workspaceId}`

  if (trimTrailingSlash(pathname) !== overviewPath) {
    return <Outlet />
  }

  return <WorkspaceOverviewPage orgSlug={orgSlug} workspaceId={workspaceId} />
}

type StatTile = {
  section: string
  label: string
  description: string
  icon: AppIcon
  count?: number
  required: readonly Permission[]
}

type NavItem = {
  section: string
  label: string
  description: string
  icon: AppIcon
  required: readonly Permission[]
}

function WorkspaceOverviewPage({ orgSlug, workspaceId }: { orgSlug: string; workspaceId: string }) {
  useWorkspacePageTitle('Overview')
  const workspace = useQuery(orgWorkspaceQueryOptions(orgSlug, workspaceId))
  const effectivePermissions = useQuery(
    orgEffectivePermissionsQueryOptions(orgSlug, 'workspace', workspaceId),
  )
  const setupStatus = useSetupStatus()
  const desktopMode = setupStatus.data?.deployment_mode === 'desktop'
  const permissions = effectivePermissions.data?.permissions

  const allStatTiles: StatTile[] = [
    {
      section: 'environments',
      label: 'Environments',
      description: 'Deployment targets for this workspace.',
      icon: 'database',
      count: workspace.data?.environment_count,
      required: workspaceEnvironmentPagePermissions,
    },
    {
      section: 'connections',
      label: 'Connections',
      description: 'Databases reachable from this workspace.',
      icon: 'flow-connection',
      count: workspace.data?.connection_count,
      required: workspaceConnectionPagePermissions,
    },
  ]
  const allNavItems: NavItem[] = [
    {
      section: 'users',
      label: 'Members',
      description: 'People and teams with workspace access.',
      icon: 'user-multiple',
      required: workspacePolicyPagePermissions,
    },
    {
      section: 'policies',
      label: 'Policies',
      description: 'Roles and access policies in this workspace.',
      icon: 'user-shield-01',
      required: workspacePolicyPagePermissions,
    },
    {
      section: 'settings',
      label: 'Settings',
      description: 'Workspace name and configuration.',
      icon: 'settings-02',
      required: workspaceSettingsPagePermissions,
    },
  ]
  const statTiles = allStatTiles.filter((tile) => hasAnyPermission(permissions, tile.required))
  const navItems = allNavItems.filter(
    (item) =>
      (!desktopMode || (item.section !== 'users' && item.section !== 'policies')) &&
      hasAnyPermission(permissions, item.required),
  )

  if (workspace.isLoading || effectivePermissions.isLoading) {
    return (
      <div className="flex flex-col gap-8">
        <Skeleton className="h-8 w-64" />
        <div className="grid gap-4 sm:grid-cols-2">
          <Skeleton className="h-28" />
          <Skeleton className="h-28" />
        </div>
        <Skeleton className="h-40" />
      </div>
    )
  }

  if (workspace.isError) {
    const notFound = isApiError(workspace.error) && workspace.error.status === 404
    return (
      <EmptyState
        icon={notFound ? 'search-01' : 'information-circle'}
        message={notFound ? 'Workspace not found' : "Couldn't load this workspace"}
        description={
          notFound
            ? 'This workspace may have been deleted, or the link you followed is incorrect.'
            : errorMessage(workspace.error, 'Something went wrong. Try again.')
        }
        action={
          <Button
            variant="outline"
            size="sm"
            nativeButton={false}
            render={<Link to="/orgs/$org_slug/workspaces" params={{ org_slug: orgSlug }} />}
          >
            <Icon name="arrow-left-01" size={16} data-icon="inline-start" />
            Back to Workspaces
          </Button>
        }
      />
    )
  }

  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex flex-col gap-1.5">
          <h1 className="font-heading text-2xl font-semibold tracking-tight">
            {workspace.data?.name ?? 'Workspace'}
          </h1>
          {workspace.data?.description ? (
            <p className="text-sm text-muted-foreground">{workspace.data.description}</p>
          ) : null}
        </div>
        <Button
          nativeButton={false}
          className="bg-primary text-primary-foreground hover:bg-primary/90"
          render={
            <Link
              to="/orgs/$org_slug/workspaces/$workspace_id/ide"
              params={{ org_slug: orgSlug, workspace_id: workspaceId }}
            />
          }
        >
          <Icon name="terminal" size={20} data-icon="inline-start" />
          Open in Editor
        </Button>
      </div>

      {statTiles.length > 0 ? (
        <div className="grid gap-4 sm:grid-cols-2">
          {statTiles.map((tile) => (
            <Link
              key={tile.section}
              to={`/orgs/$org_slug/workspaces/$workspace_id/${tile.section}` as never}
              params={{ org_slug: orgSlug, workspace_id: workspaceId } as never}
              className="group relative flex flex-col gap-4 overflow-hidden rounded-lg border border-border bg-card p-5 text-card-foreground transition-all hover:border-foreground/20 hover:shadow-sm"
            >
              <span className="absolute inset-y-0 left-0 w-0.5 scale-y-0 bg-primary transition-transform group-hover:scale-y-100" />
              <div className="flex items-center justify-between">
                <p className="text-sm font-medium text-muted-foreground">{tile.label}</p>
                <Icon
                  name={tile.icon}
                  size={18}
                  className="text-muted-foreground transition-colors group-hover:text-primary"
                />
              </div>
              <p className="font-heading text-4xl font-semibold tracking-tight tabular-nums">
                {tile.count ?? '—'}
              </p>
              <p className="text-xs leading-relaxed text-muted-foreground">{tile.description}</p>
            </Link>
          ))}
        </div>
      ) : null}

      {navItems.length > 0 ? (
        <div className="flex flex-col divide-y divide-border rounded-lg border border-border bg-card">
          {navItems.map((item) => (
            <Link
              key={item.section}
              to={`/orgs/$org_slug/workspaces/$workspace_id/${item.section}` as never}
              params={{ org_slug: orgSlug, workspace_id: workspaceId } as never}
              className="group flex items-center gap-3 p-4 text-card-foreground transition-colors first:rounded-t-lg last:rounded-b-lg hover:bg-muted/40"
            >
              <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                <Icon name={item.icon} size={18} />
              </div>
              <div className="min-w-0 flex-1">
                <p className="truncate font-medium leading-tight tracking-tight transition-colors group-hover:text-primary">
                  {item.label}
                </p>
                <p className="mt-0.5 truncate text-xs leading-relaxed text-muted-foreground">
                  {item.description}
                </p>
              </div>
              <Icon
                name="arrow-right-01"
                size={16}
                className="shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5"
              />
            </Link>
          ))}
        </div>
      ) : null}
    </div>
  )
}
