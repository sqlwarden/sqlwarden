import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Workspace } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { createIdeStore, IdeStoreContext, type EditorTab } from '../useIdeStore'
import { SchemaDiagramView } from './SchemaDiagramView'

vi.mock('idb-keyval', () => ({
  del: vi.fn(() => Promise.resolve()),
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
}))

const workspace: Workspace = {
  id: 3, org_id: 1, owner_type: 'org', owner_id: 1, name: 'Analytics',
  environment_count: 1, connection_count: 1, created_at: '', updated_at: '',
}

const tab: EditorTab = {
  id: 'diagram:7:ns:public',
  workspaceId: 3,
  title: 'public diagram',
  kind: 'diagram',
  content: '',
  connectionId: 7,
  driver: 'postgres',
  diagramTarget: { kind: 'namespace', namespace: 'public' },
}

describe('SchemaDiagramView', () => {
  let store: ReturnType<typeof createIdeStore>

  beforeEach(() => {
    store = createIdeStore('acme', 1, 'ephemeral')
  })

  function renderDiagram(editorTab: EditorTab = tab) {
    return {
      user: userEvent.setup(),
      ...render(
        <QueryClientProvider client={createTestQueryClient()}>
          <IdeStoreContext.Provider value={store}>
            <SchemaDiagramView orgSlug="acme" workspace={workspace} tab={editorTab} />
          </IdeStoreContext.Provider>
        </QueryClientProvider>,
      ),
    }
  }

  function schemaHandlers(options: { supportsDiagram?: boolean; forbidden?: boolean } = {}) {
    const base = '/api/v1/orgs/acme/workspaces/3/connections/7/schema'
    server.use(
      http.get(`${base}/spec`, () => options.forbidden
        ? HttpResponse.json({ error: { message: 'Forbidden' } }, { status: 403 })
        : HttpResponse.json({ spec: {
          dialect: 'postgres',
          kinds: options.supportsDiagram === false ? [] : [{
            kind: 'table', label: 'Table', plural_label: 'Tables', order: 1,
            relational: true, supports_diagram: true, listing: 'enumerated',
          }],
        } })),
      http.get(`${base}/catalog`, () => HttpResponse.json({
        catalog: { generated_at: '', namespaces: [{ name: 'public', groups: [] }] },
      })),
      http.get(`${base}/relationships`, () => HttpResponse.json({
        graph: { namespace: 'public', relationships: [] },
      })),
    )
  }

  it('explains a malformed diagram tab without issuing schema requests', () => {
    renderDiagram({ ...tab, connectionId: undefined, diagramTarget: undefined })
    expect(screen.getByText('This tab is missing its diagram target.')).toBeInTheDocument()
  })

  it('reconnects a stale diagram tab and stores the new session', async () => {
    schemaHandlers({ supportsDiagram: false })
    server.use(http.post(
      '/api/v1/orgs/acme/workspaces/3/connections/7/connect',
      () => HttpResponse.json({ session_id: 'session-new' }),
    ))
    const { user } = renderDiagram()

    expect(screen.getByText('postgres · connection not available')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Reconnect' }))

    await waitFor(() => expect(store.getState().sessions[7]).toBe('session-new'))
    expect(await screen.findByText("Diagrams aren't available for this connection.")).toBeInTheDocument()
    expect(store.getState().connectionStatus[7]).toBeUndefined()
  })

  it('shows unsupported, forbidden, and empty schema states', async () => {
    store.getState().setSession(7, 'session-7')
    schemaHandlers({ supportsDiagram: false })
    const unsupported = renderDiagram()
    expect(await screen.findByText("Diagrams aren't available for this connection.")).toBeInTheDocument()
    unsupported.unmount()

    schemaHandlers({ forbidden: true })
    const forbidden = renderDiagram()
    expect(await screen.findByText('You no longer have access to this connection.')).toBeInTheDocument()
    forbidden.unmount()

    schemaHandlers()
    renderDiagram()
    expect(await screen.findByText('No tables to diagram in this schema.')).toBeInTheDocument()
  })
})
