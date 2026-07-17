import type { ComponentType } from 'react'
import type { OptionalFeature } from '#/lib/product/optional-features'

export type ExtensionPageKey = 'platform-overview'
export type ExtensionSlotKey = 'login-sso-providers'

export interface ExtensionModule {
  pages: Partial<Record<ExtensionPageKey, ComponentType>>
  slots: Partial<Record<ExtensionSlotKey, ComponentType>>
  LockedFeature?: ComponentType<{ feature: OptionalFeature }>
}
