import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as Y from 'yjs'
import type { Workspace } from '#/lib/api/types'
import type { GroupNode } from './ideLayout'
import type { EditorTab } from './useIdeStore'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
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
  SqlEditor: (props: { tabId: string; groupId: string; onCursorChange?: unknown }) => {
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

describe('EditorGroup', () => {
  beforeEach(() => {
    mocks.fileState.isLoading = false
    mocks.fileState.isError = false
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
  })

  it('renders an empty group and focuses it on pointer interaction', () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const rendered = render(
      <IdeStoreContext.Provider value={store}>
        <EditorGroup orgSlug="acme" workspace={workspace} group={group} focused={false} />
      </IdeStoreContext.Provider>,
    )

    expect(screen.getByText('No editor in this group')).toBeInTheDocument()
    fireEvent.mouseDown(rendered.container.firstElementChild!)
    expect(store.getState().activeGroupId[3]).toBe('main')
  })

  it('routes editor tabs through loading, error, and ready states', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const active = tab('file')
    active.fileId = 9
    store.setState({ tabs: [active] })
    const activeGroup = { ...group, tabIds: [active.id], activeTabId: active.id }
    const user = userEvent.setup()
    mocks.fileState.isLoading = true

    const rendered = render(
      <IdeStoreContext.Provider value={store}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={activeGroup}
          focused
          onCursorChange={vi.fn()}
        />
      </IdeStoreContext.Provider>,
    )
    expect(screen.getByText('Loading…')).toBeInTheDocument()
    expect(mocks.getOrCreate).toHaveBeenCalledWith(active.id, undefined)

    mocks.fileState.isLoading = false
    mocks.fileState.isError = true
    rendered.rerender(
      <IdeStoreContext.Provider value={store}>
        <EditorGroup orgSlug="acme" workspace={workspace} group={activeGroup} focused />
      </IdeStoreContext.Provider>,
    )
    await user.click(screen.getByRole('button', { name: 'Retry' }))
    expect(mocks.retry).toHaveBeenCalled()

    mocks.fileState.isError = false
    rendered.rerender(
      <IdeStoreContext.Provider value={store}>
        <EditorGroup orgSlug="acme" workspace={workspace} group={activeGroup} focused />
      </IdeStoreContext.Provider>,
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
    const store = createIdeStore('acme', 1, 'ephemeral')
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
      <IdeStoreContext.Provider value={store}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={{ ...group, tabIds: [csvTab.id, sqlTab.id], activeTabId: csvTab.id }}
          focused
        />
      </IdeStoreContext.Provider>,
    )
    expect(screen.getByTestId('csv-viewer')).toBeInTheDocument()
    expect(screen.queryByTestId('sql-editor')).not.toBeInTheDocument()

    rendered.rerender(
      <IdeStoreContext.Provider value={store}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={{ ...group, tabIds: [csvTab.id, sqlTab.id], activeTabId: sqlTab.id }}
          focused
        />
      </IdeStoreContext.Provider>,
    )
    expect(screen.getByTestId('sql-editor')).toBeInTheDocument()
    expect(screen.queryByTestId('csv-viewer')).not.toBeInTheDocument()
  })

  it('shows the CSV loading skeleton while a CSV tab hydrates', () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const csvTab = tab('file')
    csvTab.fileId = 12
    csvTab.title = 'export.csv'
    store.setState({ tabs: [csvTab] })
    mocks.fileState.isLoading = true

    render(
      <IdeStoreContext.Provider value={store}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={{ ...group, tabIds: [csvTab.id], activeTabId: csvTab.id }}
          focused
        />
      </IdeStoreContext.Provider>,
    )
    expect(screen.getByLabelText('Loading CSV')).toBeInTheDocument()
  })

  it('does not hydrate oversized CSV files and offers a download instead', async () => {
    const store = createIdeStore('acme', 1, 'ephemeral')
    const csvTab = tab('file')
    csvTab.fileId = 12
    csvTab.title = 'large-export.csv'
    csvTab.fileSizeBytes = MAX_BROWSER_CSV_BYTES + 1
    store.setState({ tabs: [csvTab] })
    const blob = new Blob(['large csv'])
    mocks.downloadFile.mockResolvedValue(blob)
    const user = userEvent.setup()

    render(
      <IdeStoreContext.Provider value={store}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={{ ...group, tabIds: [csvTab.id], activeTabId: csvTab.id }}
          focused
        />
      </IdeStoreContext.Provider>,
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
    const store = createIdeStore('acme', 1, 'ephemeral')
    const objectTab = tab('object')
    const diagramTab = tab('diagram')
    store.setState({ tabs: [objectTab, diagramTab] })
    const rendered = render(
      <IdeStoreContext.Provider value={store}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={{ ...group, tabIds: [objectTab.id, diagramTab.id], activeTabId: objectTab.id }}
          focused
        />
      </IdeStoreContext.Provider>,
    )
    expect(screen.getByTestId('object-detail')).toHaveTextContent(objectTab.id)

    rendered.rerender(
      <IdeStoreContext.Provider value={store}>
        <EditorGroup
          orgSlug="acme"
          workspace={workspace}
          group={{ ...group, tabIds: [objectTab.id, diagramTab.id], activeTabId: diagramTab.id }}
          focused
        />
      </IdeStoreContext.Provider>,
    )
    expect(await screen.findByTestId('diagram')).toHaveTextContent(diagramTab.id)
    expect(mocks.getOrCreate).not.toHaveBeenCalled()
  })
})
