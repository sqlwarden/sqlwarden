import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { queryKeys } from '#/lib/api/query-keys'
import type { InstanceEdition } from '#/lib/api/types'
import { ENTERPRISE_FEATURES } from '#/lib/enterprise/features'
import type { EnterpriseModule } from '#/lib/enterprise/module'
import { server } from '#/test/server'
import userEvent from '@testing-library/user-event'
import { EnterpriseSlot } from './enterprise-slot'

const mockModule: EnterpriseModule = {
  pages: {},
  slots: {},
}

vi.mock('@enterprise', () => ({
  get enterpriseModule() {
    return mockModule
  },
}))

function renderSlot(edition?: InstanceEdition, fallback?: ReactNode, enabled = false) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, enabled } } })
  if (edition) client.setQueryData(queryKeys.instanceEdition(), edition)
  return render(
    <QueryClientProvider client={client}>
      <EnterpriseSlot
        slot="login-sso-providers"
        feature={ENTERPRISE_FEATURES.enterpriseSso}
        fallback={fallback}
      />
    </QueryClientProvider>,
  )
}

describe('EnterpriseSlot', () => {
  it('renders the registered implementation only when the feature is licensed', () => {
    mockModule.slots = { 'login-sso-providers': () => <div>SSO buttons</div> }

    renderSlot({ edition: 'enterprise', licensed_features: ['enterprise_sso'] })
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

  it('does not fetch edition data when this build has no slot implementation', () => {
    mockModule.slots = {}

    renderSlot(undefined, <div>fallback</div>, true)
    expect(screen.getByText('fallback')).toBeInTheDocument()
  })

  it('shows loading and recovers from an edition endpoint failure', async () => {
    mockModule.slots = { 'login-sso-providers': () => <div>SSO buttons</div> }
    let attempts = 0
    server.use(
      http.get('/api/v1/instance/edition', () => {
        attempts++
        if (attempts === 1) {
          return HttpResponse.json(
            { error: { code: 'unavailable', message: 'Unavailable' } },
            { status: 503 },
          )
        }
        return HttpResponse.json({
          edition: 'enterprise',
          licensed_features: [ENTERPRISE_FEATURES.enterpriseSso],
        })
      }),
    )

    renderSlot(undefined, undefined, true)
    expect(screen.getByText('Loading additional sign-in options…')).toBeInTheDocument()
    const retry = await screen.findByRole('button', {
      name: 'Unable to load additional sign-in options. Retry',
    })
    await userEvent.click(retry)
    expect(await screen.findByText('SSO buttons')).toBeInTheDocument()
  })
})
