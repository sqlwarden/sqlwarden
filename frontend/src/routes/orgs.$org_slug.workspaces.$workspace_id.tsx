import { trimTrailingSlash } from '#/lib/utils'
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

type OverviewCard = {
  section: string
  label: string
  description: string
  icon: AppIcon
  count?: number
  required: readonly Permission[]
}

function WorkspaceOverviewPage({ orgSlug, workspaceId }: { orgSlug: string; workspaceId: string }) {
  const workspace = useQuery(orgWorkspaceQueryOptions(orgSlug, workspaceId))
  const effectivePermissions = useQuery(
    orgEffectivePermissionsQueryOptions(orgSlug, 'workspace', workspaceId),
  )
  const permissions = effectivePermissions.data?.permissions

  const allCards: OverviewCard[] = [
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
  const cards = allCards.filter((card) => hasAnyPermission(permissions, card.required))

  if (workspace.isLoading || effectivePermissions.isLoading) {
    return (
      <div className="flex flex-col gap-6">
        <Skeleton className="h-8 w-64" />
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-28" />
          ))}
        </div>
      </div>
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
          render={
            <Link
              to="/ide/$org_slug"
              params={{ org_slug: orgSlug }}
              search={{ ws: Number(workspaceId) }}
            />
          }
        >
          <Icon name="terminal" size={20} data-icon="inline-start" />
          Open in IDE
        </Button>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {cards.map((card) => (
          <Link
            key={card.section}
            to={`/orgs/$org_slug/workspaces/$workspace_id/${card.section}` as never}
            params={{ org_slug: orgSlug, workspace_id: workspaceId } as never}
            className="group flex flex-col rounded-lg border border-border bg-card text-card-foreground transition-all hover:border-foreground/20 hover:bg-muted/20 hover:shadow-sm"
          >
            <div className="flex flex-1 items-start gap-3 p-5">
              <div className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
                <Icon name={card.icon} size={20} />
              </div>
              <div className="min-w-0 flex-1 pt-0.5">
                <div className="flex items-center gap-2">
                  <p className="truncate font-semibold leading-tight tracking-tight transition-colors group-hover:text-primary">
                    {card.label}
                  </p>
                  {card.count !== undefined ? (
                    <span className="text-xs tabular-nums text-muted-foreground">{card.count}</span>
                  ) : null}
                </div>
                <p className="mt-1.5 text-xs leading-relaxed text-muted-foreground">
                  {card.description}
                </p>
              </div>
            </div>
          </Link>
        ))}
      </div>
    </div>
  )
}
