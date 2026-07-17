import type { ComponentType, PropsWithChildren } from 'react'
import type { RouteComponent } from '@tanstack/react-router'
import type { AppShellNavItem } from '#/components/app-shell-navigation'

export type DistributionRouteScope = 'root' | 'account' | 'instance' | 'organization' | 'workspace'
export type DistributionNavigationScope = 'account' | 'instance' | 'organization' | 'workspace'

export interface DistributionRoute {
  scope: DistributionRouteScope
  path: string
  component: RouteComponent
}

export interface DistributionNavigationContext {
  orgSlug?: string
  workspaceId?: string
  permissions?: readonly string[]
}

export interface DistributionNavigation {
  scope: DistributionNavigationScope
  section?: string
  permission?: string
  item: AppShellNavItem | ((context: DistributionNavigationContext) => AppShellNavItem)
}

export interface FrontendDependencies {
  providers?: ComponentType<PropsWithChildren>[]
  routes?: DistributionRoute[]
  navigation?: DistributionNavigation[]
}
