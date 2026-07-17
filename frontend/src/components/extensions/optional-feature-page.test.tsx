import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { queryKeys } from '#/lib/api/query-keys'
import type { ExtensionModule } from '#/lib/extensions/module'
import { OPTIONAL_FEATURES } from '#/lib/product/optional-features'
import { OptionalFeaturePage } from './optional-feature-page'

const mockModule: ExtensionModule = {
  pages: {},
  slots: {},
}

vi.mock('@extensions', () => ({
  get extensionModule() {
    return mockModule
  },
}))

function renderPage(capabilities?: string[]) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, enabled: false } } })
  if (capabilities) {
    client.setQueryData(queryKeys.instanceCapabilities(), { capabilities })
  }
  return render(
    <QueryClientProvider client={client}>
      <OptionalFeaturePage pageKey="platform-overview" feature={OPTIONAL_FEATURES.auditLog} />
    </QueryClientProvider>,
  )
}

describe('OptionalFeaturePage', () => {
  it('renders public upgrade information when the build has no implementation', () => {
    mockModule.pages = {}

    renderPage()
    expect(screen.getByText('Audit logging')).toBeInTheDocument()
    expect(screen.getByText('Enterprise')).toBeInTheDocument()
    expect(screen.getByText(/not included in this build/)).toBeInTheDocument()
  })

  it('renders the extension-owned locked state when its capability is unavailable', () => {
    mockModule.pages = { 'platform-overview': () => <div>audit viewer</div> }
    mockModule.LockedFeature = () => <div>extension locked state</div>

    renderPage([])
    expect(screen.queryByText('audit viewer')).not.toBeInTheDocument()
    expect(screen.getByText('extension locked state')).toBeInTheDocument()
  })

  it('renders the implementation when its capability is available', () => {
    mockModule.pages = { 'platform-overview': () => <div>audit viewer</div> }

    renderPage([OPTIONAL_FEATURES.auditLog.id])
    expect(screen.getByText('audit viewer')).toBeInTheDocument()
  })

  it('shows loading while a compiled implementation checks availability', () => {
    mockModule.pages = { 'platform-overview': () => <div>audit viewer</div> }

    renderPage()
    expect(screen.getByText('Loading…')).toBeInTheDocument()
  })
})
