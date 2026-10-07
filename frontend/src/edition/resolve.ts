import type { ComponentType } from 'react'
import type { EditionCapabilities, EditionFeatureCapability } from '#/lib/api/types'
import type { EditionRegistry } from './types'

export type EditionPage =
  | { kind: 'route'; component: ComponentType }
  | { kind: 'locked'; feature: EditionFeatureCapability }
  | { kind: 'not-found' }

export function findEditionFeature(
  capabilities: EditionCapabilities,
  key: string,
): EditionFeatureCapability | undefined {
  return capabilities.features.find((feature) => feature.key === key)
}

// A registered page renders only while its feature is available. Any other
// path that names a catalog feature renders that feature's locked page.
export function resolveEditionPage(
  registry: EditionRegistry,
  capabilities: EditionCapabilities,
  path: string,
): EditionPage {
  const route = registry.routes.find((candidate) => candidate.path === path)
  const feature = findEditionFeature(capabilities, route?.feature ?? path)
  if (!feature) return { kind: 'not-found' }
  if (feature.state !== 'available') return { kind: 'locked', feature }
  return route ? { kind: 'route', component: route.component } : { kind: 'not-found' }
}

export interface LockedEditionNavigationItem {
  key: string
  label: string
  placement: string
  to: string
}

export function lockedEditionNavigation(
  capabilities: EditionCapabilities,
): LockedEditionNavigationItem[] {
  return capabilities.features.flatMap((feature) =>
    feature.state !== 'available' && feature.navigation
      ? [
          {
            key: feature.key,
            label: feature.label,
            placement: feature.navigation,
            to: `/ee/${feature.key}`,
          },
        ]
      : [],
  )
}
