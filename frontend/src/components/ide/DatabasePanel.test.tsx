import { QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { delay, http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { DatabasePanel } from './DatabasePanel'
import { ContextMenuProvider } from '#/components/ui/context-menu'
import { ConnectionLayoutProvider } from './useConnectionLayout'

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
    localStorage.clear()
  })

  function handlers(items: 'empty' | 'populated', environmentPermissions: string[] = []) {
    const environmentPermissionRequests = vi.fn()
    server.use(
      http.get('/api/v1/orgs/acme/permissions/effective', ({ request }) => {
        const url = new URL(request.url)
        const isEnvironment = url.searchParams.get('resource_type') === 'environment'
        if (isEnvironment) environmentPermissionRequests()
        return HttpResponse.json({
          resource_type: isEnvironment ? 'environment' : 'workspace',
          resource_id: isEnvironment ? 2 : 3,
          permissions: isEnvironment ? environmentPermissions : [],
        })
      }),
      http.get('/api/v1/orgs/acme/workspaces/3/environments', () =>
        HttpResponse.json({
          items:
            items === 'populated'
              ? [{ id: 2, workspace_id: 3, name: 'Production', created_at: '', updated_at: '' }]
              : [],
          page: 1,
          page_size: 100,
          total: items === 'populated' ? 1 : 0,
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections', () =>
        HttpResponse.json({
          items:
            items === 'populated'
              ? [
                  {
                    id: 7,
                    workspace_id: 3,
                    environment_id: 2,
                    name: 'analytics-pg',
                    driver: 'postgres',
                    access_mode: 'open',
                    created_at: '',
                    updated_at: '',
                  },
                ]
              : [],
          page: 1,
          page_size: 100,
          total: items === 'populated' ? 1 : 0,
        }),
      ),
    )
    return environmentPermissionRequests
  }

  function renderPanel(layout: 'flat' | 'grouped' = 'flat') {
    localStorage.setItem('sqlwarden.preference.connection_layout', layout)
    const rootRoute = createRootRoute({
      component: () => (
        <IdeStoreContext.Provider value={store}>
          <ConnectionLayoutProvider>
            <ContextMenuProvider>
              <DatabasePanel orgSlug="acme" workspace={workspace} />
            </ContextMenuProvider>
          </ConnectionLayoutProvider>
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
        HttpResponse.json({ resource_type: 'workspace', resource_id: 3, permissions: [] }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/environments', () =>
        HttpResponse.json({ error: { message: 'Unavailable' } }, { status: 500 }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections', () =>
        HttpResponse.json({ error: { message: 'Unavailable' } }, { status: 500 }),
      ),
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

  it('runs a connection schema refresh through the backend endpoint', async () => {
    handlers('populated')
    store.getState().setSession(7, 'session-7')
    let refreshes = 0
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/refresh', () => {
        refreshes++
        return HttpResponse.json({
          status: 'ok',
          mode: 'persistent',
          snapshot_id: 'snapshot-2',
          generated_at: '2026-08-06T00:00:00Z',
        })
      }),
    )
    const { user } = renderPanel()

    await screen.findByText('analytics-pg')
    await user.click(screen.getByRole('button', { name: 'Refresh schema' }))

    await waitFor(() => expect(refreshes).toBe(1))
  })

  it('opens connection creation from an environment quick action', async () => {
    handlers('populated', ['conn:create'])
    const { user } = renderPanel('grouped')

    const environment = await screen.findByText('Production')
    await waitFor(() => expect(screen.getByLabelText('New connection in Production')).toBeVisible())
    fireEvent.contextMenu(environment)
    await user.click(await screen.findByRole('menuitem', { name: 'New connection here' }))

    expect(await screen.findByRole('heading', { name: 'Choose a database' })).toBeInTheDocument()
  })

  it('renames an environment while preserving its description', async () => {
    const environmentPermissionRequests = handlers('populated', ['env:write'])
    let requestBody: unknown
    server.use(
      http.patch('/api/v1/orgs/acme/workspaces/3/environments/2', async ({ request }) => {
        requestBody = await request.json()
        return new HttpResponse(null, { status: 204 })
      }),
      http.get('/api/v1/orgs/acme/workspaces/3/environments', () =>
        HttpResponse.json({
          items: [
            {
              id: 2,
              workspace_id: 3,
              name: 'Production',
              description: 'Customer traffic',
              created_at: '',
              updated_at: '',
            },
          ],
          page: 1,
          page_size: 100,
          total: 1,
        }),
      ),
    )
    const { user } = renderPanel('grouped')

    const environment = await screen.findByText('Production')
    await waitFor(() => expect(environmentPermissionRequests).toHaveBeenCalled())
    fireEvent.contextMenu(environment)
    await user.click(await screen.findByRole('menuitem', { name: 'Rename environment' }))
    const name = await screen.findByRole('textbox', { name: 'Name' })
    expect(name).toHaveValue('Production')
    await user.clear(name)
    await user.type(name, 'Primary')
    await user.click(screen.getByRole('button', { name: 'Rename' }))

    await waitFor(() =>
      expect(requestBody).toEqual({ name: 'Primary', description: 'Customer traffic' }),
    )
  })

  it('blocks deleting an environment that still has connections', async () => {
    const environmentPermissionRequests = handlers('populated', ['env:delete'])
    const { user } = renderPanel('grouped')

    const environment = await screen.findByText('Production')
    await waitFor(() => expect(environmentPermissionRequests).toHaveBeenCalled())
    fireEvent.contextMenu(environment)
    await user.click(await screen.findByRole('menuitem', { name: 'Delete environment' }))

    expect(await screen.findByText(/contains 1 connection/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Delete' })).toBeDisabled()
  })

  it('deletes an empty environment and removes it from the explorer', async () => {
    let deleted = false
    let deleteCalls = 0
    const environmentPermissionRequests = vi.fn()
    server.use(
      http.get('/api/v1/orgs/acme/permissions/effective', ({ request }) => {
        const url = new URL(request.url)
        const isEnvironment = url.searchParams.get('resource_type') === 'environment'
        if (isEnvironment) environmentPermissionRequests()
        return HttpResponse.json({
          resource_type: isEnvironment ? 'environment' : 'workspace',
          resource_id: isEnvironment ? 2 : 3,
          permissions: isEnvironment ? ['env:delete'] : [],
        })
      }),
      http.get('/api/v1/orgs/acme/workspaces/3/environments', () =>
        HttpResponse.json({
          items: deleted
            ? []
            : [{ id: 2, workspace_id: 3, name: 'Production', created_at: '', updated_at: '' }],
          page: 1,
          page_size: 100,
          total: deleted ? 0 : 1,
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections', () =>
        HttpResponse.json({ items: [], page: 1, page_size: 100, total: 0 }),
      ),
      http.delete('/api/v1/orgs/acme/workspaces/3/environments/2', () => {
        deleteCalls++
        deleted = true
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { user } = renderPanel('grouped')

    const environment = await screen.findByText('Production')
    await waitFor(() => expect(environmentPermissionRequests).toHaveBeenCalled())
    fireEvent.contextMenu(environment)
    await user.click(await screen.findByRole('menuitem', { name: 'Delete environment' }))
    await user.click(await screen.findByRole('button', { name: 'Delete' }))

    await waitFor(() => expect(deleteCalls).toBe(1))
    expect(await screen.findByText('No environments available.')).toBeInTheDocument()
  })
})
