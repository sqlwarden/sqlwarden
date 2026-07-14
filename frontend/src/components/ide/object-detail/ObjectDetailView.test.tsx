import { QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ObjectDetail, ObjectRef, Workspace } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { createIdeStore, IdeStoreContext, type EditorTab } from '../useIdeStore'
import { ObjectDetailView } from './ObjectDetailView'

vi.mock('idb-keyval', () => ({
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
  del: vi.fn(() => Promise.resolve()),
}))

const workspace: Workspace = {
  id: 3,
  org_id: 1,
  owner_type: 'org',
  owner_id: 1,
  name: 'Analytics',
  environment_count: 0,
  connection_count: 1,
  created_at: '',
  updated_at: '',
}
const ref: ObjectRef = { namespace: 'public', kind: 'table', name: 'orders' }
const tab: EditorTab = {
  id: 'object:7:public:table:orders',
  workspaceId: 3,
  title: 'orders',
  kind: 'object',
  content: '',
  connectionId: 7,
  driver: 'postgres',
  objectRef: ref,
}
const detail: ObjectDetail = {
  ref,
  relational: {
    columns: [{ name: 'id', data_type: 'bigint', nullable: false, ordinal: 1 }],
    primary_key: ['id'],
    foreign_keys: [],
    indexes: [],
  },
}

describe('ObjectDetailView', () => {
  let store: ReturnType<typeof createIdeStore>

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
  })

  function renderView(editorTab = tab) {
    return render(
      <QueryClientProvider client={createTestQueryClient()}>
        <IdeStoreContext.Provider value={store}>
          <ObjectDetailView orgSlug="acme" workspace={workspace} tab={editorTab} />
        </IdeStoreContext.Provider>
      </QueryClientProvider>,
    )
  }

  function respondReady() {
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/objects', () =>
        HttpResponse.json({ objects: [detail] }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/spec', () =>
        HttpResponse.json({
          spec: {
            dialect: 'postgres',
            kinds: [
              {
                kind: 'table',
                label: 'Table',
                plural_label: 'Tables',
                order: 1,
                relational: true,
                supports_diagram: true,
                listing: 'enumerated',
              },
            ],
          },
        }),
      ),
    )
  }

  it('reports a malformed object tab without issuing schema requests', () => {
    renderView({ ...tab, objectRef: undefined })
    expect(screen.getByText('This tab is missing its object reference.')).toBeInTheDocument()
  })

  it('reconnects a stale tab and stores the replacement session', async () => {
    respondReady()
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/connect', () =>
        HttpResponse.json({ session_id: 'session-7' }),
      ),
    )
    renderView()

    fireEvent.click(screen.getByRole('button', { name: 'Reconnect' }))

    await waitFor(() => expect(store.getState().sessions[7]).toBe('session-7'))
    expect(await screen.findByRole('button', { name: 'Columns' })).toBeInTheDocument()
  })

  it('renders relational sections and opens a supported object diagram', async () => {
    store.getState().setSession(7, 'session-7')
    respondReady()
    renderView()

    expect(await screen.findByRole('button', { name: 'Columns' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Keys & Indexes' })).toBeInTheDocument()
    expect(screen.getByText('id')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'View in diagram' }))
    expect(store.getState().tabs).toEqual(
      expect.arrayContaining([expect.objectContaining({ kind: 'diagram', connectionId: 7 })]),
    )
  })

  it('renders permission loss without stale object data', async () => {
    store.getState().setSession(7, 'session-7')
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/objects', () =>
        HttpResponse.json(
          {
            error: { code: 'forbidden', message: 'Forbidden' },
          },
          { status: 403 },
        ),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections/7/schema/spec', () =>
        HttpResponse.json({ spec: { dialect: 'postgres', kinds: [] } }),
      ),
    )
    renderView()

    expect(
      await screen.findByText('You no longer have access to this connection.'),
    ).toBeInTheDocument()
  })
})
