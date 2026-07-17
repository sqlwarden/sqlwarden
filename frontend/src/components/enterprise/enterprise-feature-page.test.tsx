import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { queryKeys } from '#/lib/api/query-keys'
import type { InstanceEdition } from '#/lib/api/types'
import type { EnterpriseModule } from '#/lib/enterprise/module'
import { EnterpriseFeaturePage } from './enterprise-feature-page'

const mockModule: EnterpriseModule = {
  edition: 'enterprise',
  pages: {},
  slots: {},
}

vi.mock('@enterprise', () => ({
  get enterpriseModule() {
    return mockModule
  },
}))

function renderPage(edition?: InstanceEdition) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, enabled: false } } })
  if (edition) {
    client.setQueryData(queryKeys.instanceEdition(), edition)
  }
  return render(
    <QueryClientProvider client={client}>
      <EnterpriseFeaturePage
        pageKey="enterprise-overview"
        feature="audit_log"
        title="Audit logs"
        description="Every security-relevant action, recorded."
      />
    </QueryClientProvider>,
  )
}

describe('EnterpriseFeaturePage', () => {
  it('renders the upsell when the module provides no page (community build)', () => {
    mockModule.pages = {}

    renderPage({ edition: 'community', licensed_features: [] })
    expect(screen.getByText('Audit logs')).toBeInTheDocument()
    expect(screen.getByText('Enterprise')).toBeInTheDocument()
    expect(screen.getByText('Every security-relevant action, recorded.')).toBeInTheDocument()
    expect(screen.getByText(/part of SQLWarden Enterprise/)).toBeInTheDocument()
  })

  it('renders the apply-key upsell on an unlicensed enterprise server even when the page is registered', () => {
    mockModule.pages = { 'enterprise-overview': () => <div>real audit viewer</div> }

    renderPage({ edition: 'enterprise', licensed_features: [] })
    expect(screen.queryByText('real audit viewer')).not.toBeInTheDocument()
    expect(screen.getByText(/Apply a license key/)).toBeInTheDocument()
  })

  it('renders the real page when registered and licensed', () => {
    mockModule.pages = { 'enterprise-overview': () => <div>real audit viewer</div> }

    renderPage({ edition: 'enterprise', licensed_features: ['audit_log'] })
    expect(screen.getByText('real audit viewer')).toBeInTheDocument()
    expect(screen.queryByText(/Apply a license key/)).not.toBeInTheDocument()
  })
})
