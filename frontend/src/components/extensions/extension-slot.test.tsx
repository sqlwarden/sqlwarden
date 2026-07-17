import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { ExtensionModule } from '#/lib/extensions/module'
import { OPTIONAL_FEATURES } from '#/lib/product/optional-features'
import { queryKeys } from '#/lib/api/query-keys'
import { server } from '#/test/server'
import { ExtensionSlot } from './extension-slot'

const mockModule: ExtensionModule = {
  pages: {},
  slots: {},
}

vi.mock('@extensions', () => ({
  get extensionModule() {
    return mockModule
  },
}))

function renderSlot(capabilities?: string[], fallback?: ReactNode, enabled = false) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, enabled } } })
  if (capabilities) {
    client.setQueryData(queryKeys.instanceCapabilities(), { capabilities })
  }
  return render(
    <QueryClientProvider client={client}>
      <ExtensionSlot
        slot="login-sso-providers"
        feature={OPTIONAL_FEATURES.enterpriseSso}
        fallback={fallback}
      />
    </QueryClientProvider>,
  )
}

describe('ExtensionSlot', () => {
  it('renders the implementation when its capability is available', () => {
    mockModule.slots = { 'login-sso-providers': () => <div>SSO buttons</div> }

    renderSlot([OPTIONAL_FEATURES.enterpriseSso.id])
    expect(screen.getByText('SSO buttons')).toBeInTheDocument()
  })

  it('renders the fallback when a compiled slot is locked', () => {
    mockModule.slots = { 'login-sso-providers': () => <div>SSO buttons</div> }

    renderSlot([], <div>locked</div>)
    expect(screen.queryByText('SSO buttons')).not.toBeInTheDocument()
    expect(screen.getByText('locked')).toBeInTheDocument()
  })

  it('does not fetch capabilities when this build has no slot implementation', () => {
    mockModule.slots = {}

    renderSlot(undefined, <div>fallback</div>, true)
    expect(screen.getByText('fallback')).toBeInTheDocument()
  })

  it('shows loading and recovers from a capability endpoint failure', async () => {
    mockModule.slots = { 'login-sso-providers': () => <div>SSO buttons</div> }
    let attempts = 0
    server.use(
      http.get('/api/v1/instance/capabilities', () => {
        attempts++
        if (attempts === 1) {
          return HttpResponse.json(
            { error: { code: 'unavailable', message: 'Unavailable' } },
            { status: 503 },
          )
        }
        return HttpResponse.json({ capabilities: [OPTIONAL_FEATURES.enterpriseSso.id] })
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
