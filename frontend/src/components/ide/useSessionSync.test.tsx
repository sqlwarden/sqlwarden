import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { delay, http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it } from 'vitest'
import type { Workspace } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { useSessionSync } from './useSessionSync'

const workspace: Workspace = {
  id: 3,
  org_id: 1,
  owner_type: 'org',
  owner_id: 1,
  name: 'Analytics',
  environment_count: 1,
  connection_count: 2,
  created_at: '',
  updated_at: '',
}

describe('useSessionSync', () => {
  let store: ReturnType<typeof createIdeStore>

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
    store.setState({ sessions: { 7: 'stale', 99: 'other-workspace' } })
  })

  function wrapper({ children }: PropsWithChildren) {
    return (
      <QueryClientProvider client={createTestQueryClient()}>
        <IdeStoreContext.Provider value={store}>{children}</IdeStoreContext.Provider>
      </QueryClientProvider>
    )
  }

  function connectionsResponse() {
    return HttpResponse.json({
      items: [
        {
          id: 7,
          workspace_id: 3,
          environment_id: 1,
          name: 'old',
          driver: 'postgres',
          access_mode: 'open',
          created_at: '',
          updated_at: '',
        },
        {
          id: 8,
          workspace_id: 3,
          environment_id: 1,
          name: 'new',
          driver: 'postgres',
          access_mode: 'open',
          created_at: '',
          updated_at: '',
        },
      ],
      page: 1,
      page_size: 100,
      total: 2,
    })
  }

  it('reconciles only sessions belonging to the current workspace', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections', connectionsResponse),
      http.get('/api/v1/orgs/acme/workspaces/3/sessions', () =>
        HttpResponse.json({
          sessions: [{ connection_id: 8, session_id: 'session-8' }],
        }),
      ),
    )

    renderHook(() => useSessionSync('acme', workspace), { wrapper })

    await waitFor(() =>
      expect(store.getState().sessions).toEqual({
        8: 'session-8',
        99: 'other-workspace',
      }),
    )
  })

  it('waits for both authoritative inputs before changing persisted state', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/connections', async () => {
        await delay(80)
        return connectionsResponse()
      }),
      http.get('/api/v1/orgs/acme/workspaces/3/sessions', () =>
        HttpResponse.json({ sessions: [] }),
      ),
    )

    renderHook(() => useSessionSync('acme', workspace), { wrapper })
    expect(store.getState().sessions).toEqual({ 7: 'stale', 99: 'other-workspace' })
    await waitFor(() => expect(store.getState().sessions).toEqual({ 99: 'other-workspace' }))
  })
})
