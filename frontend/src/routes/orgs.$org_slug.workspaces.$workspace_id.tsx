import { Outlet, createFileRoute, useRouterState } from '@tanstack/react-router'

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

  return <PlaceholderPage title="Overview" />
}

function PlaceholderPage({ title }: { title: string }) {
  return (
    <div className="flex flex-col gap-2">
      <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
      <p className="text-sm text-muted-foreground">{title} works!</p>
    </div>
  )
}

function trimTrailingSlash(path: string) {
  return path === '/' ? path : path.replace(/\/$/, '')
}

