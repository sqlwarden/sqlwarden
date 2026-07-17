import type { EnterpriseModule } from '#/lib/enterprise/module'

// Community builds alias '@enterprise' to this stub: no pages registered,
// so every enterprise route renders its upsell state.
export const enterpriseModule: EnterpriseModule = {
  edition: 'community',
  pages: {},
}
