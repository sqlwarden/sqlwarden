import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import * as Y from 'yjs'
import type { Workspace } from '#/lib/api/types'
import type { GroupNode } from './ideLayout'
import type { EditorTab } from './useIdeStore'
import { createIdeStore, IdeStoreContext } from './useIdeStore'
import { EditorGroup } from './EditorGroup'

const mocks = vi.hoisted(() => ({
  fileState: { isLoading: false, isError: false },
  getOrCreate: vi.fn(),
  retry: vi.fn(),
  sqlEditor: vi.fn(),
}))

vi.mock('./useFileContent', () => ({
  useFileContent: () => ({ ...mocks.fileState, retry: mocks.retry }),
}))
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
  ObjectDetailView: ({ tab }: { tab: { id: string } }) => <div data-testid="object-detail">{tab.id}</div>,
}))
vi.mock('./schema-diagram/SchemaDiagramView', () => ({
  SchemaDiagramView: ({ tab }: { tab: { id: string } }) => <div data-testid="diagram">{tab.id}</div>,
}))

const workspace: Workspace = {
  id: 3, org_id: 1, owner_type: 'org', owner_id: 1, name: 'Analytics',
  environment_count: 0, connection_count: 0, created_at: '', updated_at: '',
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
        <EditorGroup orgSlug="acme" workspace={workspace} group={activeGroup} focused onCursorChange={vi.fn()} />
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
    expect(mocks.sqlEditor).toHaveBeenLastCalledWith(expect.objectContaining({
      tabId: active.id, groupId: 'main',
    }))
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
