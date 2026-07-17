import type { PropsWithChildren } from 'react'
import { createRoute, type AnyRoute } from '@tanstack/react-router'
import { AppShellNavSection } from '#/components/app-shell'
import { distribution } from './build'
import type {
  DistributionNavigationContext,
  DistributionNavigationScope,
  DistributionRouteScope,
} from './types'

export function DistributionProviders({ children }: PropsWithChildren) {
  return (distribution.providers ?? []).reduceRight(
    (content, Provider) => <Provider>{content}</Provider>,
    children,
  )
}

export function composeDistributionRoutes(parents: Record<DistributionRouteScope, AnyRoute>) {
  const children = new Map<AnyRoute, AnyRoute[]>()
  for (const contribution of distribution.routes ?? []) {
    const parent = parents[contribution.scope]
    const route = createRoute({
      getParentRoute: () => parent,
      path: contribution.path,
      component: contribution.component,
    })
    children.set(parent, [...(children.get(parent) ?? []), route])
  }
  for (const [parent, routes] of children) parent.addChildren(routes)
}

interface DistributionNavigationSectionsProps extends DistributionNavigationContext {
  scope: DistributionNavigationScope
  pathname: string
}

export function DistributionNavigationSections({
  scope,
  pathname,
  orgSlug,
  workspaceId,
  permissions,
}: DistributionNavigationSectionsProps) {
  const groups = distributionNavigationGroups(scope, { orgSlug, workspaceId, permissions })
  return [...groups].map(([section, items]) => (
    <AppShellNavSection
      key={section || 'distribution'}
      label={section || undefined}
      items={items}
      pathname={pathname}
    />
  ))
}

export function distributionNavigationGroups(
  scope: DistributionNavigationScope,
  context: DistributionNavigationContext,
) {
  const groups = new Map<string, ReturnType<typeof resolveNavigationItem>[]>()
  for (const contribution of distribution.navigation ?? []) {
    if (contribution.scope !== scope) continue
    if (contribution.permission && !context.permissions?.includes(contribution.permission)) continue
    const section = contribution.section ?? ''
    groups.set(section, [
      ...(groups.get(section) ?? []),
      resolveNavigationItem(contribution.item, context),
    ])
  }
  return groups
}

function resolveNavigationItem(
  item: NonNullable<NonNullable<typeof distribution.navigation>[number]>['item'],
  context: DistributionNavigationContext,
) {
  return typeof item === 'function' ? item(context) : item
}
