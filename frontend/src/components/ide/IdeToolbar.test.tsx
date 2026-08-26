import { QueryClientProvider } from '@tanstack/react-query'
import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Workspace } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { IdeToolbar } from './IdeToolbar'
import { createIdeStore, IdeStoreContext, type EditorTab } from './useIdeStore'
import { createEditorViewRegistry, EditorViewRegistryContext } from './useEditorViewRegistry'

const mocks = vi.hoisted(() => ({
  cancel: vi.fn(),
  confirmAt: vi.fn(() => Promise.resolve()),
  download: vi.fn(),
  resolveDocumentText: vi.fn(() => ''),
  resolveSql: vi.fn(() => 'select 1'),
  run: vi.fn(() => Promise.resolve()),
  runAll: vi.fn(() => Promise.resolve()),
  save: vi.fn(() => Promise.resolve(undefined)),
  isRunning: false,
}))

vi.mock('./useToolbarQueryAction', () => ({
  useToolbarQueryAction: () => ({
    cancel: mocks.cancel,
    confirmAt: mocks.confirmAt,
    isRunning: mocks.isRunning,
    resolveDocumentText: mocks.resolveDocumentText,
    resolveSql: mocks.resolveSql,
    run: mocks.run,
    runAll: mocks.runAll,
  }),
}))

vi.mock('./useSaveEditorTab', () => ({ useSaveEditorTab: () => mocks.save }))
vi.mock('./exports/useDownloadNow', () => ({
  useDownloadNow: () => ({
    bytesDownloaded: 0,
    cancel: vi.fn(),
    download: mocks.download,
    isDownloading: false,
  }),
}))

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

const scratchTab: EditorTab = {
  id: 'scratch:3:1',
  workspaceId: 3,
  title: 'Console 1',
  kind: 'scratch',
  content: 'select 1',
  connectionId: 7,
  driver: 'postgres',
}

describe('IdeToolbar', () => {
  let store: ReturnType<typeof createIdeStore>
  let views: ReturnType<typeof createEditorViewRegistry>

  beforeEach(() => {
    vi.clearAllMocks()
    mocks.isRunning = false
    store = createIdeStore('acme', 1, 'ephemeral')
    views = createEditorViewRegistry()
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/environments', () =>
        HttpResponse.json({
          items: [{ id: 2, workspace_id: 3, name: 'Production', created_at: '', updated_at: '' }],
          page: 1,
          page_size: 100,
          total: 1,
        }),
      ),
      http.get('/api/v1/orgs/acme/workspaces/3/connections', () =>
        HttpResponse.json({
          items: [
            {
              id: 7,
              workspace_id: 3,
              environment_id: 2,
              name: 'primary-pg',
              driver: 'postgres',
              access_mode: 'open',
              created_at: '',
              updated_at: '',
            },
            {
              id: 8,
              workspace_id: 3,
              environment_id: 2,
              name: 'warehouse',
              driver: 'mysql',
              access_mode: 'open',
              created_at: '',
              updated_at: '',
            },
          ],
          page: 1,
          page_size: 100,
          total: 2,
        }),
      ),
    )
  })

  function renderToolbar() {
    return {
      user: userEvent.setup(),
      ...render(
        <QueryClientProvider client={createTestQueryClient()}>
          <IdeStoreContext.Provider value={store}>
            <EditorViewRegistryContext.Provider value={views}>
              <IdeToolbar orgSlug="acme" workspace={workspace} />
            </EditorViewRegistryContext.Provider>
          </IdeStoreContext.Provider>
        </QueryClientProvider>,
      ),
    }
  }

  it('disables editor actions and hides save and export actions without an active tab', async () => {
    renderToolbar()

    expect(screen.getByRole('button', { name: /^Run/ })).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Save file' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'More run options' })).not.toBeInTheDocument()
    expect(await screen.findByRole('button', { name: /Select connection/ })).toBeDisabled()
  })

  it('runs, saves, and exposes export actions for a SQL tab', async () => {
    store.getState().openTab(scratchTab)
    const { user } = renderToolbar()

    await waitFor(() => expect(screen.getByRole('button', { name: /^Run/ })).toBeEnabled())
    await user.click(screen.getByRole('button', { name: /^Run/ }))
    await user.click(screen.getByRole('button', { name: 'Save file' }))

    expect(mocks.run).toHaveBeenCalledOnce()
    expect(mocks.save).toHaveBeenCalledWith(scratchTab)
    expect(screen.getByRole('button', { name: 'More run options' })).toBeEnabled()
    expect(screen.getByRole('button', { name: 'Format SQL' })).toBeEnabled()
  })

  it('formats the active editor using its connection dialect', async () => {
    store.getState().openTab(scratchTab)
    const groupId = store.getState().activeGroupId[workspace.id]!
    const editor = new EditorView({
      state: EditorState.create({ doc: 'select * from users where id=1' }),
    })
    views.register(`${groupId}:${scratchTab.id}`, editor)
    const { user } = renderToolbar()

    await user.click(screen.getByRole('button', { name: 'Format SQL' }))

    expect(editor.state.doc.toString()).toBe('select\n  *\nfrom\n  users\nwhere\n  id = 1')
    editor.destroy()
  })

  it('hides export for non-SQL files and disables save until a file is dirty', async () => {
    const csvTab: EditorTab = {
      id: 'file:11',
      workspaceId: 3,
      title: 'report.csv',
      kind: 'file',
      fileId: 11,
      content: 'id,name',
      etag: 'one',
      isDirty: false,
      connectionId: 7,
    }
    store.getState().openTab(csvTab)
    const view = renderToolbar()

    await waitFor(() => expect(screen.getByRole('button', { name: /^Run/ })).toBeEnabled())
    expect(screen.queryByRole('button', { name: 'More run options' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Save file' })).toBeDisabled()
    expect(screen.queryByRole('button', { name: 'Format SQL' })).not.toBeInTheDocument()

    store.setState({ tabs: [{ ...csvTab, isDirty: true }] })
    expect(await screen.findByRole('button', { name: 'Save file' })).toBeEnabled()
    view.unmount()
  })

  it('shows cancellation state while a query runs', async () => {
    store.getState().openTab(scratchTab)
    mocks.isRunning = true
    const { user } = renderToolbar()

    expect(screen.getByRole('button', { name: /Running/ })).toBeDisabled()
    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(mocks.cancel).toHaveBeenCalledOnce()
  })

  it('assigns a selected connection to the active tab', async () => {
    store.getState().openTab(scratchTab)
    const { user } = renderToolbar()

    await user.click(await screen.findByRole('button', { name: /primary-pg/ }))
    await user.click(screen.getByRole('button', { name: /warehouse/ }))

    expect(store.getState().tabs[0]).toEqual(
      expect.objectContaining({ connectionId: 8, driver: 'mysql' }),
    )
  })

  it('repairs a persisted console that has a connection but no driver', async () => {
    store.getState().openTab({ ...scratchTab, driver: undefined })
    renderToolbar()

    await waitFor(() =>
      expect(store.getState().tabs.find((tab) => tab.id === scratchTab.id)).toMatchObject({
        connectionId: 7,
        driver: 'postgres',
      }),
    )
  })

  it('toggles editor maximization', async () => {
    const { user } = renderToolbar()
    const toggle = screen.getByRole('button', { name: 'Toggle editor maximize' })

    await user.click(toggle)
    expect(store.getState().maximizedPane).toBe('editor')
    await user.click(toggle)
    expect(store.getState().maximizedPane).toBeNull()
  })

  it('splits the document and runs every statement via Run All', async () => {
    store.getState().openTab(scratchTab)
    mocks.resolveDocumentText.mockReturnValue('select 1;\nselect 2;')
    const { user } = renderToolbar()

    await waitFor(() => expect(screen.getByRole('button', { name: /^Run/ })).toBeEnabled())
    await user.click(screen.getByRole('button', { name: 'More run options' }))
    await user.click(await screen.findByRole('menuitem', { name: 'Run All' }))

    expect(mocks.runAll).toHaveBeenCalledWith(['select 1', 'select 2'])
  })

  it('resumes a paused run confirmation via confirmAt and clears it on confirm', async () => {
    store.getState().openTab(scratchTab)
    const runId = store.getState().beginRun(scratchTab.id, ['select 1', 'DELETE FROM widgets'])
    store.getState().setPendingConfirmation(scratchTab.id, {
      sql: 'DELETE FROM widgets',
      statements: [],
      runId,
      statementIndex: 1,
    })
    const { user } = renderToolbar()

    await user.click(screen.getByRole('button', { name: 'Run Anyway' }))

    expect(mocks.confirmAt).toHaveBeenCalledWith(1)
    expect(mocks.run).not.toHaveBeenCalled()
    expect(store.getState().pendingConfirmations[scratchTab.id]).toBeUndefined()
  })

  it('abandons the rest of a run when a pending confirmation is dismissed', async () => {
    store.getState().openTab(scratchTab)
    const runId = store
      .getState()
      .beginRun(scratchTab.id, ['select 1', 'DELETE FROM widgets', 'select 3'])
    store.getState().setPendingConfirmation(scratchTab.id, {
      sql: 'DELETE FROM widgets',
      statements: [],
      runId,
      statementIndex: 1,
    })
    const { user } = renderToolbar()

    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    expect(store.getState().pendingConfirmations[scratchTab.id]).toBeUndefined()
    const results = store.getState().resultRuns[scratchTab.id][0].results
    expect(results[1]).toEqual({
      status: 'skipped',
      sql: 'DELETE FROM widgets',
    })
    expect(results[2]).toEqual({ status: 'skipped', sql: 'select 3' })
  })
})
