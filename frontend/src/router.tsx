import { createRouter as createTanStackRouter, type RouterHistory } from '@tanstack/react-router'
import { routeTree } from './routeTree.gen'

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
