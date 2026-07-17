import { createRouter as createTanStackRouter, type RouterHistory } from '@tanstack/react-router'
import { routeTree } from './routeTree.gen'
import { composeDistributionRoutes } from '#/distribution/composition'
import { Route as SettingsRoute } from './routes/settings'
import { Route as AdministrationRoute } from './routes/administration'
import { Route as OrganizationRoute } from './routes/orgs.$org_slug'
import { Route as WorkspaceRoute } from './routes/orgs.$org_slug.workspaces.$workspace_id'

composeDistributionRoutes({
  root: routeTree,
  account: SettingsRoute,
  instance: AdministrationRoute,
  organization: OrganizationRoute,
  workspace: WorkspaceRoute,
})

interface RouterOptions {
  history?: RouterHistory
}

export function getRouter(options: RouterOptions = {}) {
  const router = createTanStackRouter({
    routeTree,
    history: options.history,
    scrollRestoration: true,
    defaultPreload: 'intent',
    defaultPreloadStaleTime: 0,
    defaultPendingMs: 0,
    defaultPendingMinMs: 200,
  })

  return router
}

declare module '@tanstack/react-router' {
  interface Register {
    router: ReturnType<typeof getRouter>
  }
}
