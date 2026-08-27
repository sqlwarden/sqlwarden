import { QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as Y from 'yjs'
import type { Workspace } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import type { GroupNode } from './ideLayout'
import type { EditorTab } from './useIdeStore'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { createEditorViewRegistry, EditorViewRegistryContext } from './useEditorViewRegistry'
import { EditorGroup } from './EditorGroup'
import { MAX_BROWSER_CSV_BYTES } from './csv/csvFile'

const mocks = vi.hoisted(() => ({
  fileState: { isLoading: false, isError: false },
  getOrCreate: vi.fn(),
  retry: vi.fn(),
  sqlEditor: vi.fn(),
  fileContentHook: vi.fn(),
  downloadFile: vi.fn(),
  saveBlobAs: vi.fn(),
  cancel: vi.fn(),
  closeExplainAnalyzeConfirm: vi.fn(),
  confirmAt: vi.fn(() => Promise.resolve()),
  confirmExplainAnalyze: vi.fn(),
  handleExplainAnalyzeClick: vi.fn(),
  handleExplainClick: vi.fn(),
  resolveDocumentText: vi.fn(() => ''),
  resolveSql: vi.fn(() => 'select 1'),
  run: vi.fn(() => Promise.resolve()),
  runAll: vi.fn(() => Promise.resolve()),
  isRunning: false,
  canExplain: true,
  canExplainAnalyze: true,
  explainAnalyzeConfirmSql: null as string | null,
}))

vi.mock('./useFileContent', () => ({
  useFileContent: (options: unknown) => {
    mocks.fileContentHook(options)
    return { ...mocks.fileState, retry: mocks.retry }
  },
}))
vi.mock('#/lib/api/files', () => ({
  downloadPrivateWorkspaceFile: (...args: unknown[]) => mocks.downloadFile(...args),
}))
vi.mock('./saveFile', () => ({ saveBlobAs: (...args: unknown[]) => mocks.saveBlobAs(...args) }))
vi.mock('./useYDocRegistry', () => ({
  useYDocRegistry: () => ({ getOrCreate: mocks.getOrCreate }),
}))
vi.mock('./IdeTabBar', () => ({ IdeTabBar: () => <div data-testid="tab-bar" /> }))
vi.mock('./SqlEditor', () => ({
  SqlEditor: (props: {
    tabId: string
    groupId: string
    onCursorChange?: unknown
    contextMenu?: unknown
  }) => {
    mocks.sqlEditor(props)
    return <div data-testid="sql-editor">{props.tabId}</div>
  },
}))
vi.mock('./object-detail/ObjectDetailView', () => ({
  ObjectDetailView: ({ tab }: { tab: { id: string } }) => (
    <div data-testid="object-detail">{tab.id}</div>
  ),
}))
vi.mock('./schema-diagram/SchemaDiagramView', () => ({
  SchemaDiagramView: ({ tab }: { tab: { id: string } }) => (
    <div data-testid="diagram">{tab.id}</div>
  ),
}))
vi.mock('./csv/CsvViewer', () => ({
  CsvViewer: ({ doc }: { doc: { getText: (key: string) => { toString: () => string } } }) => (
    <div data-testid="csv-viewer">{doc.getText('content').toString()}</div>
  ),
}))
vi.mock('./useToolbarQueryAction', () => ({
  useToolbarQueryAction: () => ({
    cancel: mocks.cancel,
    canExplain: mocks.canExplain,
    canExplainAnalyze: mocks.canExplainAnalyze,
    closeExplainAnalyzeConfirm: mocks.closeExplainAnalyzeConfirm,
    confirmAt: mocks.confirmAt,
    confirmExplainAnalyze: mocks.confirmExplainAnalyze,
    explainAnalyzeConfirmSql: mocks.explainAnalyzeConfirmSql,
    handleExplainAnalyzeClick: mocks.handleExplainAnalyzeClick,
    handleExplainClick: mocks.handleExplainClick,
    isRunning: mocks.isRunning,
    resolveDocumentText: mocks.resolveDocumentText,
    resolveSql: mocks.resolveSql,
    run: mocks.run,
    runAll: mocks.runAll,
  }),
}))

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
const group: GroupNode = { type: 'group', id: 'main', tabIds: [], activeTabId: undefined }

function tab(kind: EditorTab['kind']): EditorTab {
  return { id: `${kind}:1`, workspaceId: 3, title: kind, kind, content: 'select 1' }
}

function Providers({
  store,
  views,
  children,
}: {
  store: ReturnType<typeof createIdeStore>
  views: ReturnType<typeof createEditorViewRegistry>
  children: React.ReactNode
}) {
  return (
    <QueryClientProvider client={createTestQueryClient()}>
      <IdeStoreContext.Provider value={store}>
        <EditorViewRegistryContext.Provider value={views}>
          {children}
        </EditorViewRegistryContext.Provider>
      </IdeStoreContext.Provider>
    </QueryClientProvider>
  )
}

describe('EditorGroup', () => {
  let store: ReturnType<typeof createIdeStore>
  let views: ReturnType<typeof createEditorViewRegistry>

  beforeEach(() => {
    mocks.fileState.isLoading = false
    mocks.fileState.isError = false
    mocks.isRunning = false
    mocks.canExplain = true
    mocks.canExplainAnalyze = true
    mocks.explainAnalyzeConfirmSql = null
    mocks.getOrCreate.mockReset().mockImplementation((_id: string, content?: string) => {
      const doc = new Y.Doc()
      if (content) doc.getText('content').insert(0, content)
      return doc
    })
    mocks.retry.mockReset()
    mocks.sqlEditor.mockReset()
    mocks.fileContentHook.mockReset()
    mocks.downloadFile.mockReset()
    mocks.saveBlobAs.mockReset()
    store = createIdeStore('acme', 1, 'ephemeral')
    views = createEditorViewRegistry()
    server.use(
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
          ],
          page: 1,
          page_size: 100,
          total: 1,
        }),
      ),
      http.get('/api/v1/engines/postgres', () =>
        HttpResponse.json({
          id: 'postgres',
          display_name: 'PostgreSQL',
          capabilities: { 'sql.explain': true },
          explain: { supports_analyze: true },
        }),
      ),
    )
  })

  it('renders an empty group and focuses it on pointer interaction', () => {
    const rendered = render(
      <Providers store={store} views={views}>
        <EditorGroup orgSlug="acme" workspace={workspace} group={group} focused={false} />
      </Providers>,
    )

    expect(screen.getByText('No editor in this group')).toBeInTheDocument()
    fireEvent.mouseDown(rendered.container.firstElementChild!)
    expect(store.getState().activeGroupId[3]).toBe('main')
  })

  it('routes editor tabs through loading, error, and ready states', async () => {
    const active = tab('file')
    active.fileId = 9
    store.setState({ tabs: [active] })
    const activeGroup = { ...group, tabIds: [active.id], activeTabId: active.id }
    const user = userEvent.setup()
    mocks.fileState.isLoading = true

    const rendered = render(
      <Providers store={store} views={views}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={activeGroup}
          focused
          onCursorChange={vi.fn()}
        />
      </Providers>,
    )
    expect(screen.getByText('Loading…')).toBeInTheDocument()
    expect(mocks.getOrCreate).toHaveBeenCalledWith(active.id, undefined)

    mocks.fileState.isLoading = false
    mocks.fileState.isError = true
    rendered.rerender(
      <Providers store={store} views={views}>
        <EditorGroup orgSlug="acme" workspace={workspace} group={activeGroup} focused />
      </Providers>,
    )
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(mocks.retry).toHaveBeenCalled()

    mocks.fileState.isError = false
    rendered.rerender(
      <Providers store={store} views={views}>
        <EditorGroup orgSlug="acme" workspace={workspace} group={activeGroup} focused />
      </Providers>,
    )
    expect(screen.getByTestId('sql-editor')).toHaveTextContent(active.id)
    expect(mocks.sqlEditor).toHaveBeenLastCalledWith(
      expect.objectContaining({
        tabId: active.id,
        groupId: 'main',
      }),
    )
  })

  it('routes CSV file tabs to CsvViewer and regular files to SqlEditor', () => {
    const csvTab = tab('file')
    csvTab.id = 'file:csv'
    csvTab.title = 'export.csv'
    csvTab.fileId = 10
    csvTab.fileMediaType = 'text/csv'
    const sqlTab = tab('file')
    sqlTab.id = 'file:sql'
    sqlTab.title = 'query.sql'
    sqlTab.fileId = 11
    store.setState({ tabs: [csvTab, sqlTab] })

    const rendered = render(
      <Providers store={store} views={views}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={{ ...group, tabIds: [csvTab.id, sqlTab.id], activeTabId: csvTab.id }}
          focused
        />
      </Providers>,
    )
    expect(screen.getByTestId('csv-viewer')).toBeInTheDocument()
    expect(screen.queryByTestId('sql-editor')).not.toBeInTheDocument()

    rendered.rerender(
      <Providers store={store} views={views}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={{ ...group, tabIds: [csvTab.id, sqlTab.id], activeTabId: sqlTab.id }}
          focused
        />
      </Providers>,
    )
    expect(screen.getByTestId('sql-editor')).toBeInTheDocument()
    expect(screen.queryByTestId('csv-viewer')).not.toBeInTheDocument()
  })

  it('shows the CSV loading skeleton while a CSV tab hydrates', () => {
    const csvTab = tab('file')
    csvTab.fileId = 12
    csvTab.title = 'export.csv'
    store.setState({ tabs: [csvTab] })
    mocks.fileState.isLoading = true

    render(
      <Providers store={store} views={views}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={{ ...group, tabIds: [csvTab.id], activeTabId: csvTab.id }}
          focused
        />
      </Providers>,
    )
    expect(screen.getByLabelText('Loading CSV')).toBeInTheDocument()
  })

  it('does not hydrate oversized CSV files and offers a download instead', async () => {
    const csvTab = tab('file')
    csvTab.fileId = 12
    csvTab.title = 'large-export.csv'
    csvTab.fileSizeBytes = MAX_BROWSER_CSV_BYTES + 1
    store.setState({ tabs: [csvTab] })
    const blob = new Blob(['large csv'])
    mocks.downloadFile.mockResolvedValue(blob)
    const user = userEvent.setup()

    render(
      <Providers store={store} views={views}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={{ ...group, tabIds: [csvTab.id], activeTabId: csvTab.id }}
          focused
        />
      </Providers>,
    )

    expect(screen.getByText('CSV is too large to preview')).toBeInTheDocument()
    expect(screen.getByText(/Browser previews are limited to 10 MB/)).toBeInTheDocument()
    expect(mocks.fileContentHook).toHaveBeenLastCalledWith(
      expect.objectContaining({ enabled: false }),
    )
    expect(mocks.getOrCreate).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: 'Download CSV' }))
    await waitFor(() => {
      expect(mocks.downloadFile).toHaveBeenCalledWith('acme', 3, 12)
      expect(mocks.saveBlobAs).toHaveBeenCalledWith('large-export.csv', blob)
    })
  })

  it('routes object and diagram tabs without allocating editor documents', async () => {
    const objectTab = tab('object')
    const diagramTab = tab('diagram')
    store.setState({ tabs: [objectTab, diagramTab] })
    const rendered = render(
      <Providers store={store} views={views}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={{ ...group, tabIds: [objectTab.id, diagramTab.id], activeTabId: objectTab.id }}
          focused
        />
      </Providers>,
    )
    expect(screen.getByTestId('object-detail')).toHaveTextContent(objectTab.id)

    rendered.rerender(
      <Providers store={store} views={views}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={{ ...group, tabIds: [objectTab.id, diagramTab.id], activeTabId: diagramTab.id }}
          focused
        />
      </Providers>,
    )
    expect(await screen.findByTestId('diagram')).toHaveTextContent(diagramTab.id)
    expect(mocks.getOrCreate).not.toHaveBeenCalled()
  })

  it('passes a context menu config that reflects tab type and connection state', async () => {
    const sqlTab = tab('scratch')
    sqlTab.connectionId = 7
    const objectTab = tab('object')
    store.setState({ tabs: [sqlTab, objectTab] })

    const rendered = render(
      <Providers store={store} views={views}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={{ ...group, tabIds: [sqlTab.id], activeTabId: sqlTab.id }}
          focused
        />
      </Providers>,
    )

    await waitFor(() => {
      expect(mocks.sqlEditor).toHaveBeenLastCalledWith(
        expect.objectContaining({
          contextMenu: expect.objectContaining({
            isSqlTab: true,
            canRun: true,
            canExplainAnalyze: true,
          }),
        }),
      )
    })

    const notSqlTab = tab('file')
    notSqlTab.title = 'notes.txt'
    notSqlTab.fileId = 20
    store.setState({ tabs: [sqlTab, objectTab, notSqlTab] })
    rendered.rerender(
      <Providers store={store} views={views}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={{ ...group, tabIds: [notSqlTab.id], activeTabId: notSqlTab.id }}
          focused
        />
      </Providers>,
    )

    await waitFor(() => {
      expect(mocks.sqlEditor).toHaveBeenLastCalledWith(
        expect.objectContaining({
          contextMenu: expect.objectContaining({
            isSqlTab: false,
          }),
        }),
      )
    })
  })

  it('disables canExplainAnalyze when the connected engine does not support EXPLAIN ANALYZE', async () => {
    mocks.canExplainAnalyze = false
    const sqlTab = tab('scratch')
    sqlTab.connectionId = 7
    store.setState({ tabs: [sqlTab] })

    render(
      <Providers store={store} views={views}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={{ ...group, tabIds: [sqlTab.id], activeTabId: sqlTab.id }}
          focused
        />
      </Providers>,
    )

    await waitFor(() => {
      expect(mocks.sqlEditor).toHaveBeenLastCalledWith(
        expect.objectContaining({
          contextMenu: expect.objectContaining({
            isSqlTab: true,
            canExplainAnalyze: false,
          }),
        }),
      )
    })
  })
})
