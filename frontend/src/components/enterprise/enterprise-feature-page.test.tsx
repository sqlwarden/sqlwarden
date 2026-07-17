import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { EnterpriseFeaturePage } from './enterprise-feature-page'

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, enabled: false } } })
  return render(
    <QueryClientProvider client={client}>
      <EnterpriseFeaturePage
        pageKey="enterprise-overview"
        title="Audit logs"
        description="Every security-relevant action, recorded."
      />
    </QueryClientProvider>,
  )
}

describe('EnterpriseFeaturePage', () => {
  it('renders the upsell state when the enterprise module provides no page', () => {
    // Tests resolve '@enterprise' to the stub (tsconfig path), so no page is
    // registered for the key and the upsell state must render.
    renderPage()
    expect(screen.getByText('Audit logs')).toBeInTheDocument()
    expect(screen.getByText('Enterprise')).toBeInTheDocument()
    expect(screen.getByText('Every security-relevant action, recorded.')).toBeInTheDocument()
  })
})
