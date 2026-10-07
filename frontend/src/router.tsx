import { createRouter as createTanStackRouter, type RouterHistory } from '@tanstack/react-router'
import { editionRegistry } from '@edition'
import { validateEditionRegistry } from '#/edition/types'
import { routeTree } from './routeTree.gen'

interface RouterOptions {
  history?: RouterHistory
}

export function getRouter(options: RouterOptions = {}) {
  // Validation eagerly resolves the build-selected registry. Initial
  // registries are empty; later consumers compose their typed contributions.
  validateEditionRegistry(editionRegistry)
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
