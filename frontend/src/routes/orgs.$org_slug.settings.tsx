import { Navigate, Outlet, createFileRoute, useRouterState } from '@tanstack/react-router'
import { RoutePending } from '#/components/RoutePending'

export const Route = createFileRoute('/orgs/$org_slug/settings')({
  component: OrganizationSettingsRoute,
  pendingComponent: RoutePending,
})

function OrganizationSettingsRoute() {
  const { org_slug: orgSlug } = Route.useParams()
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const settingsPath = `/orgs/${orgSlug}/settings`

  if (trimTrailingSlash(pathname) === settingsPath) {
    return <Navigate to="/orgs/$org_slug/settings/general" params={{ org_slug: orgSlug }} replace />
  }

  return <Outlet />
}

function trimTrailingSlash(path: string) {
  return path === '/' ? path : path.replace(/\/$/, '')
}
