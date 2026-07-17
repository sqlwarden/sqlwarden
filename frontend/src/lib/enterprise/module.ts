import type { ComponentType } from 'react'

export type EnterprisePageKey = 'enterprise-overview'

export interface EnterpriseModule {
  edition: 'community' | 'enterprise'
  pages: Partial<Record<EnterprisePageKey, ComponentType>>
}
