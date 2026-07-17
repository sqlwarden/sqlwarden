import type { ComponentType } from 'react'

// Page keys name whole routes whose implementation is enterprise-only. The
// route file always exists in core; the page registry decides whether the
// real page or the core upsell renders.
export type EnterprisePageKey = 'enterprise-overview'

// Slot keys name places inside shared core pages where an enterprise-only
// section renders. Add the key here (core, AGPL — the shape of the seam is
// public) and register the implementation in src/enterprise (EE builds
// only). Slots take no props: implementations fetch their own data so the
// hosting core page never depends on enterprise types.
export type EnterpriseSlotKey = 'login-sso-providers'

export interface EnterpriseModule {
  edition: 'community' | 'enterprise'
  pages: Partial<Record<EnterprisePageKey, ComponentType>>
  slots: Partial<Record<EnterpriseSlotKey, ComponentType>>
}
