import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ThemeProvider } from '#/components/theme-provider'
import type { SessionResponse, Workspace } from '#/lib/api/types'
import { getAccessToken, setAccessToken } from '#/lib/auth/access-token'
import { organizationRuntimeSettingsFixture } from '#/test/fixtures'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { IdeActivityBar } from './IdeActivityBar'
import { createIdeStore, IdeStoreContext } from './useIdeStore'

const router = vi.hoisted(() => ({ navigate: vi.fn(() => Promise.resolve()) }))
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  const React = await import('react')
  return {
    ...actual,
    Link: React.forwardRef<
      HTMLAnchorElement,
      { to: string; children?: React.ReactNode } & React.AnchorHTMLAttributes<HTMLAnchorElement>
    >(({ to, children, ...props }, ref) => (
      <a ref={ref} href={to} {...props}>
        {children}
      </a>
    )),
    useNavigate: () => router.navigate,
  }
})

function makeWorkspace(id: number, name: string): Workspace {
  return {
    id,
    org_id: 1,
    owner_type: 'org',
    owner_id: 1,
    name,
    environment_count: 0,
    connection_count: 0,
    created_at: '',
    updated_at: '',
  }
}

const session: SessionResponse = {
  account: { id: 1, name: 'Ada Lovelace', email: 'ada@example.com', is_active: true },
  organizations: [{ id: 1, slug: 'acme', name: 'Acme', created_at: '', updated_at: '' }],
  is_instance_admin: false,
  personal_spaces_enabled: false,
}

describe('IdeActivityBar', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    server.use(
      http.get('/api/v1/orgs/acme/runtime-settings', () =>
        HttpResponse.json(organizationRuntimeSettingsFixture()),
      ),
    )
  })

  it('renders activities in Search, Explorer, Files, History, Favorites, Exports order', () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const workspace = makeWorkspace(1, 'Analytics')
    render(
      <ThemeProvider disableTransitionOnChange={false}>
        <QueryClientProvider client={createTestQueryClient()}>
          <IdeStoreContext.Provider value={store}>
            <IdeActivityBar
              orgSlug="acme"
              workspaces={[workspace]}
              activeWorkspace={workspace}
              onSelectWorkspace={vi.fn()}
              session={undefined}
              canAccessOrgSettings={false}
            />
          </IdeStoreContext.Provider>
        </QueryClientProvider>
      </ThemeProvider>,
    )

    const activityButtons = screen
      .getAllByRole('button')
      .filter((button) => button.hasAttribute('aria-pressed'))
    expect(activityButtons.map((button) => button.getAttribute('aria-label'))).toEqual([
      'Search',
      'Explorer',
      'Files',
      'History',
      'Favorites',
      'Exports',
    ])
  })

  it('renders a brand link back to the dashboard', () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const workspace = makeWorkspace(1, 'Analytics')
    render(
      <ThemeProvider disableTransitionOnChange={false}>
        <QueryClientProvider client={createTestQueryClient()}>
          <IdeStoreContext.Provider value={store}>
            <IdeActivityBar
              orgSlug="acme"
              workspaces={[workspace]}
              activeWorkspace={workspace}
              onSelectWorkspace={vi.fn()}
              session={undefined}
              canAccessOrgSettings={false}
            />
          </IdeStoreContext.Provider>
        </QueryClientProvider>
      </ThemeProvider>,
    )

    expect(screen.getByRole('link', { name: /home$/ })).toHaveAttribute('href', '/')
  })

  it('is collapsed by default and expands to show activity labels, workspace, UI Lab, and account details via the toggle', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const user = userEvent.setup()
    const workspace = makeWorkspace(1, 'Analytics')
    render(
      <ThemeProvider disableTransitionOnChange={false}>
        <QueryClientProvider client={createTestQueryClient()}>
          <IdeStoreContext.Provider value={store}>
            <IdeActivityBar
              orgSlug="acme"
              workspaces={[workspace]}
              activeWorkspace={workspace}
              onSelectWorkspace={vi.fn()}
              session={session}
              canAccessOrgSettings={false}
            />
          </IdeStoreContext.Provider>
        </QueryClientProvider>
      </ThemeProvider>,
    )

    expect(store.getState().activityBarExpanded).toBe(false)
    expect(screen.queryByText('Explorer')).not.toBeInTheDocument()
    expect(screen.queryByText('Analytics')).not.toBeInTheDocument()
    expect(screen.queryByText('UI Lab')).not.toBeInTheDocument()
    expect(screen.queryByText('ada@example.com')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Toggle activity bar' }))

    expect(store.getState().activityBarExpanded).toBe(true)
    expect(screen.getByText('Explorer')).toBeInTheDocument()
    expect(screen.getByText('Analytics')).toBeInTheDocument()
    expect(screen.getByText('UI Lab')).toBeInTheDocument()
    expect(screen.getByText('Ada Lovelace')).toBeInTheDocument()
    expect(screen.getByText('ada@example.com')).toBeInTheDocument()
  })

  it('toggles the active sidebar and expands it when switching activities', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const user = userEvent.setup()
    const workspace = makeWorkspace(1, 'Analytics')
    render(
      <ThemeProvider disableTransitionOnChange={false}>
        <QueryClientProvider client={createTestQueryClient()}>
          <IdeStoreContext.Provider value={store}>
            <IdeActivityBar
              orgSlug="acme"
              workspaces={[workspace]}
              activeWorkspace={workspace}
              onSelectWorkspace={vi.fn()}
              session={undefined}
              canAccessOrgSettings={false}
            />
          </IdeStoreContext.Provider>
        </QueryClientProvider>
      </ThemeProvider>,
    )

    expect(screen.getByRole('navigation', { name: 'Editor activities' })).toBeInTheDocument()
    const explorer = screen.getByRole('button', { name: 'Explorer' })
    expect(explorer).toHaveAttribute('aria-pressed', 'true')
    await user.click(explorer)
    expect(store.getState().sidebarCollapsed).toBe(true)

    await user.click(screen.getByRole('button', { name: 'Files' }))
    expect(store.getState().activeActivityId).toBe('files')
    expect(store.getState().sidebarCollapsed).toBe(false)
  })

  it('calls onSelectWorkspace when picking a workspace from the rail selector', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const user = userEvent.setup()
    const workspaces = [makeWorkspace(1, 'Analytics'), makeWorkspace(2, 'Billing')]
    const onSelectWorkspace = vi.fn()
    render(
      <ThemeProvider disableTransitionOnChange={false}>
        <QueryClientProvider client={createTestQueryClient()}>
          <IdeStoreContext.Provider value={store}>
            <IdeActivityBar
              orgSlug="acme"
              workspaces={workspaces}
              activeWorkspace={workspaces[0]}
              onSelectWorkspace={onSelectWorkspace}
              session={undefined}
              canAccessOrgSettings={false}
            />
          </IdeStoreContext.Provider>
        </QueryClientProvider>
      </ThemeProvider>,
    )

    await user.click(screen.getByRole('button', { name: 'Analytics' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Billing' }))

    expect(onSelectWorkspace).toHaveBeenCalledWith(2)
  })

  it('shows the session account in the avatar menu and clears authentication state and redirects even when logout fails', async () => {
    server.use(
      http.post('/api/v1/auth/logout', () =>
        HttpResponse.json({ error: { message: 'Unavailable' } }, { status: 503 }),
      ),
    )
    setAccessToken('token')
    const store = createIdeStore('acme', 1, 'ephemeral')
    const queryClient = createTestQueryClient()
    queryClient.setQueryData(['permissions', 'acme'], ['org:read'])
    const user = userEvent.setup()
    const workspace = makeWorkspace(1, 'Analytics')

    render(
      <ThemeProvider disableTransitionOnChange={false}>
        <QueryClientProvider client={queryClient}>
          <IdeStoreContext.Provider value={store}>
            <IdeActivityBar
              orgSlug="acme"
              workspaces={[workspace]}
              activeWorkspace={workspace}
              onSelectWorkspace={vi.fn()}
              session={session}
              canAccessOrgSettings
            />
          </IdeStoreContext.Provider>
        </QueryClientProvider>
      </ThemeProvider>,
    )

    await user.click(screen.getByRole('button', { name: 'Ada Lovelace' }))
    expect(screen.getByText('ada@example.com')).toBeInTheDocument()
    await user.click(await screen.findByRole('menuitem', { name: 'Sign out' }))

    await waitFor(() =>
      expect(router.navigate).toHaveBeenCalledWith({ to: '/login', replace: true }),
    )
    expect(getAccessToken()).toBeNull()
    expect(queryClient.getQueryData(['permissions', 'acme'])).toBeUndefined()
  })
})
