import type { ComponentType } from 'react'

export interface EditionRouteContribution {
  id: string
  feature: string
  path: string
  component: ComponentType
}

export interface EditionNavigationContribution {
  id: string
  label: string
  to: string
}

export interface EditionSettingsContribution {
  id: string
  component: ComponentType
}

export interface EditionGateContribution {
  feature: string
  component: ComponentType
}

export interface EditionRegistry {
  routes: EditionRouteContribution[]
  navigation: EditionNavigationContribution[]
  settings: EditionSettingsContribution[]
  gates: EditionGateContribution[]
}

export function emptyEditionRegistry(): EditionRegistry {
  return { routes: [], navigation: [], settings: [], gates: [] }
}

export function validateEditionRegistry(registry: EditionRegistry): EditionRegistry {
  const ids = new Set<string>()
  const paths = new Set<string>()
  for (const contribution of [...registry.routes, ...registry.navigation, ...registry.settings]) {
    if (ids.has(contribution.id))
      throw new Error(`Duplicate edition contribution: ${contribution.id}`)
    ids.add(contribution.id)
  }
  for (const route of registry.routes) {
    if (paths.has(route.path)) throw new Error(`Duplicate edition route: ${route.path}`)
    paths.add(route.path)
  }
  return registry
}
