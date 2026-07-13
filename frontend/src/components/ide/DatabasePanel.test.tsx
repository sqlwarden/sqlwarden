import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryHistory, createRootRoute, createRouter, RouterProvider } from '@tanstack/react-router'
import { delay, http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { DatabasePanel } from './DatabasePanel'

vi.mock('idb-keyval', () => ({
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
  del: vi.fn(() => Promise.resolve()),
}))

const workspace = {
  id: 3,
  org_id: 1,
  owner_type: 'org' as const,
  owner_id: 1,
  name: 'Analytics',
  environment_count: 1,
  connection_count: 1,
  created_at: '',
  updated_at: '',
}

describe('DatabasePanel', () => {
  let store: ReturnType<typeof createIdeStore>

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
  })

  function handlers(items: 'empty' | 'populated') {
    server.use(
      http.get('/api/v1/orgs/acme/permissions/effective', () =>
        HttpResponse.json({ resource_type: 'workspace', resource_id: 3, permissions: [] })),
      http.get('/api/v1/orgs/acme/workspaces/3/environments', () =>
        HttpResponse.json({
          items: items === 'populated' ? [{ id: 2, workspace_id: 3, name: 'Production', created_at: '', updated_at: '' }] : [],
          page: 1,
          page_size: 100,
          total: items === 'populated' ? 1 : 0,
        })),
      http.get('/api/v1/orgs/acme/workspaces/3/connections', () =>
        HttpResponse.json({
          items: items === 'populated' ? [{
            id: 7,
            workspace_id: 3,
            environment_id: 2,
            name: 'analytics-pg',
            driver: 'postgres',
            access_mode: 'open',
            created_at: '',
            updated_at: '',
          }] : [],
          page: 1,
          page_size: 100,
          total: items === 'populated' ? 1 : 0,
        })),
    )
  }

  function renderPanel() {
    const rootRoute = createRootRoute({
      component: () => (
        <IdeStoreContext.Provider value={store}>
          <DatabasePanel orgSlug="acme" workspace={workspace} />
        </IdeStoreContext.Provider>
      ),
    })
    const router = createRouter({
      routeTree: rootRoute,
      history: createMemoryHistory({ initialEntries: ['/'] }),
    })
    return {
      user: userEvent.setup(),
      ...render(
        <QueryClientProvider client={createTestQueryClient()}>
          <RouterProvider router={router} />
        </QueryClientProvider>,
      ),
    }
  }

  it('shows a stable loading state while explorer queries are pending', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/permissions/effective', async () => {
        await delay(100)
        return HttpResponse.json({ resource_type: 'workspace', resource_id: 3, permissions: [] })
      }),
      http.get('/api/v1/orgs/acme/workspaces/3/environments', async () => {
        await delay(100)
        return HttpResponse.json({ items: [], page: 1, page_size: 100, total: 0 })
      }),
      http.get('/api/v1/orgs/acme/workspaces/3/connections', async () => {
        await delay(100)
        return HttpResponse.json({ items: [], page: 1, page_size: 100, total: 0 })
      }),
    )

    renderPanel()
    expect(await screen.findByText('Loading...')).toBeInTheDocument()
  })

  it('shows the empty environment state', async () => {
    handlers('empty')
    renderPanel()
    expect(await screen.findByText('No environments available.')).toBeInTheDocument()
  })

  it('offers retry when explorer requests fail', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/permissions/effective', () =>
        HttpResponse.json({ resource_type: 'workspace', resource_id: 3, permissions: [] })),
      http.get('/api/v1/orgs/acme/workspaces/3/environments', () =>
        HttpResponse.json({ error: { message: 'Unavailable' } }, { status: 500 })),
      http.get('/api/v1/orgs/acme/workspaces/3/connections', () =>
        HttpResponse.json({ error: { message: 'Unavailable' } }, { status: 500 })),
    )

    renderPanel()
    expect(await screen.findByText('Failed to load connections.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })

  it('opens a connection tab from a populated flat explorer', async () => {
    handlers('populated')
    const { user } = renderPanel()

    const connectionName = await screen.findByText('analytics-pg')
    await user.click(connectionName.closest('button')!)
    expect(store.getState().tabs).toEqual([
      expect.objectContaining({ id: 'connection:7', connectionId: 7 }),
    ])
  })
})
