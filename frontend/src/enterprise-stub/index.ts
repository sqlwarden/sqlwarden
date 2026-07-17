import type { EnterpriseModule } from '#/lib/enterprise/module'

// Community builds alias '@enterprise' to this stub: no pages or slots
// registered, so every enterprise surface renders its upsell/fallback state.
export const enterpriseModule: EnterpriseModule = {
  pages: {},
  slots: {},
}
