import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Connection, Workspace } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { useConnectionActions } from './useConnectionActions'

vi.mock('idb-keyval', () => ({
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
  del: vi.fn(() => Promise.resolve()),
}))

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

const workspace: Workspace = {
  id: 3,
  org_id: 1,
  owner_type: 'org',
  owner_id: 1,
  name: 'Analytics',
  environment_count: 1,
  connection_count: 1,
  created_at: '',
  updated_at: '',
}

const connection: Connection = {
  id: 7,
  workspace_id: 3,
  environment_id: 2,
  name: 'analytics-pg',
  driver: 'postgres',
  access_mode: 'open',
  created_at: '',
  updated_at: '',
}

describe('useConnectionActions', () => {
  let store: ReturnType<typeof createIdeStore>
  const queryClient = createTestQueryClient()

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
    queryClient.clear()
  })

  function wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={queryClient}>
        <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
      </QueryClientProvider>
    )
  }

  it('tracks connecting state and stores the returned session', async () => {
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/connect', () =>
        HttpResponse.json({ session_id: 'session-7', reused: false }),
      ),
    )
    const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
    const { result } = renderHook(() => useConnectionActions('acme', workspace), { wrapper })

    act(() => result.current.connect(connection))
    expect(store.getState().connectionStatus[7]).toBe('connecting')
    await waitFor(() => expect(store.getState().sessions[7]).toBe('session-7'))
    expect(store.getState().connectionStatus[7]).toBeUndefined()
    expect(invalidate).toHaveBeenCalledWith({
      queryKey: ['org-workspace-sessions', 'acme', 3],
    })
  })

  it('disconnects with the owning session and clears local state', async () => {
    store.getState().setSession(7, 'session-7')
    store
      .getState()
      .setTransactionState(7, { mode: 'manual', open: true, pendingStatements: 1, statements: [] })
    let requestSession = ''
    server.use(
      http.delete('/api/v1/orgs/acme/workspaces/3/connections/7/session', ({ request }) => {
        requestSession = request.headers.get('X-Warden-Session') ?? ''
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { result } = renderHook(() => useConnectionActions('acme', workspace), { wrapper })

    act(() => result.current.disconnect(connection, false))
    await waitFor(() => expect(store.getState().sessions[7]).toBeUndefined())
    expect(requestSession).toBe('session-7')
    expect(store.getState().transactions[7]).toBeUndefined()
  })

  it('does not issue a disconnect without a live session', async () => {
    let requests = 0
    server.use(
      http.delete('/api/v1/orgs/acme/workspaces/3/connections/7/session', () => {
        requests += 1
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { result } = renderHook(() => useConnectionActions('acme', workspace), { wrapper })

    act(() => result.current.disconnect(connection, false))
    await Promise.resolve()
    expect(requests).toBe(0)
  })

  it('blocks disconnect and calls onBlocked when a transaction is open, without issuing a request', async () => {
    store.getState().setSession(7, 'session-7')
    let requests = 0
    server.use(
      http.delete('/api/v1/orgs/acme/workspaces/3/connections/7/session', () => {
        requests += 1
        return new HttpResponse(null, { status: 204 })
      }),
    )
    const { result } = renderHook(() => useConnectionActions('acme', workspace), { wrapper })
    const onBlocked = vi.fn()

    act(() => result.current.disconnect(connection, true, onBlocked))
    await Promise.resolve()

    expect(onBlocked).toHaveBeenCalledTimes(1)
    expect(requests).toBe(0)
    expect(store.getState().sessions[7]).toBe('session-7')
  })

  it('opens connection and console tabs through the editor store', () => {
    const { result } = renderHook(() => useConnectionActions('acme', workspace), { wrapper })

    act(() => result.current.openConnection(connection))
    expect(store.getState().tabs).toEqual(
      expect.arrayContaining([expect.objectContaining({ id: 'connection:7', connectionId: 7 })]),
    )

    act(() => result.current.openConnectionConsole(connection))
    expect(store.getState().tabs).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          kind: 'scratch',
          connectionId: 7,
          driver: 'postgres',
        }),
      ]),
    )
  })
})
