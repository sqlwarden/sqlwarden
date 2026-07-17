import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { queryKeys } from '#/lib/api/query-keys'
import type { InstanceEdition } from '#/lib/api/types'
import type { EnterpriseModule } from '#/lib/enterprise/module'
import { EnterpriseSlot } from './enterprise-slot'

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

function renderSlot(edition: InstanceEdition, fallback?: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, enabled: false } } })
  client.setQueryData(queryKeys.instanceEdition(), edition)
  return render(
    <QueryClientProvider client={client}>
      <EnterpriseSlot slot="login-sso-providers" feature="sso" fallback={fallback} />
    </QueryClientProvider>,
  )
}

describe('EnterpriseSlot', () => {
  it('renders the registered implementation only when the feature is licensed', () => {
    mockModule.slots = { 'login-sso-providers': () => <div>SSO buttons</div> }

    renderSlot({ edition: 'enterprise', licensed_features: ['sso'] })
    expect(screen.getByText('SSO buttons')).toBeInTheDocument()
  })

  it('renders the fallback on an unlicensed enterprise server even when registered', () => {
    mockModule.slots = { 'login-sso-providers': () => <div>SSO buttons</div> }

    renderSlot({ edition: 'enterprise', licensed_features: [] }, <div>locked</div>)
    expect(screen.queryByText('SSO buttons')).not.toBeInTheDocument()
    expect(screen.getByText('locked')).toBeInTheDocument()
  })

  it('renders nothing by default when no implementation is registered', () => {
    mockModule.slots = {}

    const { container } = renderSlot({ edition: 'community', licensed_features: [] })
    expect(container).toBeEmptyDOMElement()
  })
})
