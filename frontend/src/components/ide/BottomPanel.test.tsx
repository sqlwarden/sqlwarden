import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Workspace } from '#/lib/api/types'
import { organizationRuntimeSettingsFixture } from '#/test/fixtures'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { BottomPanelHeader, BottomPanelContent } from './BottomPanel'
import { createIdeStore, IdeStoreContext } from './useIdeStore'

vi.mock('./ResultsArea', () => ({ ResultsArea: () => <div>Results content</div> }))
vi.mock('./HistoryPanel', () => ({ HistoryPanel: () => <div>History content</div> }))
vi.mock('./FavoritesPanel', () => ({ FavoritesPanel: () => <div>Favorites content</div> }))

const workspace: Workspace = {
  id: 3,
  org_id: 1,
  owner_type: 'org',
  owner_id: 1,
  name: 'Analytics',
  environment_count: 0,
  connection_count: 0,
  created_at: '',
  updated_at: '',
}

describe('BottomPanel', () => {
  let store: ReturnType<typeof createIdeStore>

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
    server.use(
      http.get('/api/v1/orgs/acme/runtime-settings', () =>
        HttpResponse.json(organizationRuntimeSettingsFixture()),
      ),
    )
  })

  function renderBottomPanel() {
    const queryClient = createTestQueryClient()
    return render(
      <QueryClientProvider client={queryClient}>
        <IdeStoreContext.Provider value={store}>
          <BottomPanelHeader orgSlug="acme" />
          <BottomPanelContent orgSlug="acme" workspace={workspace} />
        </IdeStoreContext.Provider>
      </QueryClientProvider>,
    )
  }

  it('opens to the Results tab by default', async () => {
    renderBottomPanel()
    expect(await screen.findByText('Results content')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Results' })).toHaveAttribute('aria-pressed', 'true')
  })

  it('renders the header without maximize/close when no panel is open', () => {
    store.getState().setActiveBottomPanel(null)
    renderBottomPanel()
    expect(screen.getByRole('button', { name: 'Results' })).toBeInTheDocument()
    expect(screen.queryByText('Results content')).not.toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Toggle bottom panel maximize' }),
    ).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Close panel' })).not.toBeInTheDocument()
  })

  it('switches tabs on click and persists the selection to the store', async () => {
    const user = userEvent.setup()
    renderBottomPanel()

    await user.click(await screen.findByRole('button', { name: 'History' }))
    expect(await screen.findByText('History content')).toBeInTheDocument()
    expect(store.getState().activeBottomPanelId).toBe('history')

    await user.click(screen.getByRole('button', { name: 'Favorites' }))
    expect(await screen.findByText('Favorites content')).toBeInTheDocument()
    expect(store.getState().activeBottomPanelId).toBe('favorites')
  })

  it('closes the panel when clicking its already-open tab, keeping the header', async () => {
    const user = userEvent.setup()
    renderBottomPanel()

    await user.click(screen.getByRole('button', { name: 'Results' }))
    expect(screen.queryByText('Results content')).not.toBeInTheDocument()
    expect(store.getState().activeBottomPanelId).toBeNull()
    expect(screen.getByRole('button', { name: 'Results' })).toBeInTheDocument()
  })

  it('closes the panel via the header close button and clears maximize', async () => {
    const user = userEvent.setup()
    store.getState().setMaximizedPane('results')
    renderBottomPanel()

    await user.click(screen.getByRole('button', { name: 'Close panel' }))
    expect(screen.queryByText('Results content')).not.toBeInTheDocument()
    expect(store.getState().activeBottomPanelId).toBeNull()
    expect(store.getState().maximizedPane).toBeNull()
  })

  it('hides History and Favorites tabs when their runtime mode is off', async () => {
    const fixture = organizationRuntimeSettingsFixture()
    server.use(
      http.get('/api/v1/orgs/acme/runtime-settings', () =>
        HttpResponse.json({
          ...fixture,
          effective: {
            ...fixture.effective,
            query_history_mode: 'off',
            query_favorites_mode: 'off',
          },
        }),
      ),
    )
    renderBottomPanel()

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'History' })).not.toBeInTheDocument()
    })
    expect(screen.queryByRole('button', { name: 'Favorites' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Results' })).toBeInTheDocument()
  })

  it('maximizes and restores the bottom panel', async () => {
    const user = userEvent.setup()
    renderBottomPanel()
    const toggle = screen.getByRole('button', { name: 'Toggle bottom panel maximize' })

    await user.click(toggle)
    expect(store.getState().maximizedPane).toBe('results')
    await user.click(toggle)
    expect(store.getState().maximizedPane).toBeNull()
  })
})
