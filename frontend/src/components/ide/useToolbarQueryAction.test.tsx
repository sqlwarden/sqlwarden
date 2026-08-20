import type { PropsWithChildren } from 'react'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { EditorView } from '@codemirror/view'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Connection, Workspace } from '#/lib/api/types'
import { createEditorViewRegistry, EditorViewRegistryContext } from './useEditorViewRegistry'
import { createIdeStore, IdeStoreContext, type EditorTab } from './useIdeStore'
import { createYDocRegistry, YDocRegistryContext } from './useYDocRegistry'
import { resolveEditorSql, useToolbarQueryAction } from './useToolbarQueryAction'

const mocks = vi.hoisted(() => ({
  cancel: vi.fn(),
  run: vi.fn(() => Promise.resolve()),
  warning: vi.fn(),
}))
vi.mock('./useQueryExecution', () => ({
  useQueryExecution: () => ({
    cancel: mocks.cancel,
    confirmAt: vi.fn(),
    isRunning: false,
    run: mocks.run,
  }),
}))
vi.mock('./useRunAllStatements', () => ({
  useRunAllStatements: () => ({
    runAll: vi.fn(),
    confirmAt: vi.fn(),
    cancel: vi.fn(),
    isRunning: false,
  }),
}))
vi.mock('sonner', () => ({ toast: { warning: mocks.warning } }))

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
const connection: Connection = {
  id: 7,
  workspace_id: 3,
  environment_id: 1,
  name: 'warehouse',
  driver: 'postgres',
  access_mode: 'open',
  created_at: '',
  updated_at: '',
}
const tab: EditorTab = {
  id: 'scratch:1',
  workspaceId: 3,
  title: 'Console',
  kind: 'scratch',
  content: 'select 1;\nselect 2;',
}

describe('resolveEditorSql', () => {
  it('prefers explicit editor selection and otherwise resolves the statement at the cursor', () => {
    const views = createEditorViewRegistry()
    const docs = createYDocRegistry(1, 'test-resolve')
    const selectedView = {
      state: {
        selection: { main: { from: 0, to: 8, head: 8 } },
        sliceDoc: () => 'select 1',
        doc: { toString: () => tab.content },
      },
    } as unknown as EditorView
    views.register('group-1:scratch:1', selectedView)
    expect(
      resolveEditorSql({
        activeGroupId: 'group-1',
        tab,
        viewRegistry: views,
        documentRegistry: docs,
      }),
    ).toBe('select 1')

    views.register('group-1:scratch:1', {
      state: {
        selection: { main: { from: 10, to: 10, head: 15 } },
        sliceDoc: () => '',
        doc: { toString: () => tab.content },
      },
    } as unknown as EditorView)
    expect(
      resolveEditorSql({
        activeGroupId: 'group-1',
        tab,
        viewRegistry: views,
        documentRegistry: docs,
      }),
    ).toBe('select 2;')
    docs.disposeAll()
  })

  it('falls back to the live Y document before the debounced tab snapshot', () => {
    const views = createEditorViewRegistry()
    const docs = createYDocRegistry(1, 'test-doc')
    docs.getOrCreate(tab.id, 'select 42')
    expect(resolveEditorSql({ tab, viewRegistry: views, documentRegistry: docs })).toBe('select 42')
    docs.disposeAll()
  })
})

describe('useToolbarQueryAction', () => {
  let store: ReturnType<typeof createIdeStore>
  let views: ReturnType<typeof createEditorViewRegistry>
  let docs: ReturnType<typeof createYDocRegistry>

  beforeEach(() => {
    vi.clearAllMocks()
    store = createIdeStore('acme', 1, 'ephemeral')
    store.getState().setMaximizedPane('editor')
    views = createEditorViewRegistry()
    docs = createYDocRegistry(1, `toolbar-${Math.random()}`)
  })

  function wrapper({ children }: PropsWithChildren) {
    return (
      <IdeStoreContext.Provider value={store}>
        <EditorViewRegistryContext.Provider value={views}>
          <YDocRegistryContext.Provider value={docs}>{children}</YDocRegistryContext.Provider>
        </EditorViewRegistryContext.Provider>
      </IdeStoreContext.Provider>
    )
  }

  it('runs the current SQL from the keyboard shortcut and restores the results pane', async () => {
    const { unmount } = renderHook(
      () =>
        useToolbarQueryAction({
          orgSlug: 'acme',
          workspace,
          activeTab: tab,
          activeConnection: connection,
          hasConnections: true,
        }),
      { wrapper },
    )

    act(() =>
      window.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true }),
      ),
    )

    await waitFor(() => expect(mocks.run).toHaveBeenCalledWith(tab.content))
    expect(store.getState().maximizedPane).toBeNull()
    unmount()
    docs.disposeAll()
  })

  it('explains a missing selection or an empty workspace connection list', async () => {
    const first = renderHook(
      () =>
        useToolbarQueryAction({
          orgSlug: 'acme',
          workspace,
          activeTab: tab,
          hasConnections: true,
        }),
      { wrapper },
    )
    await act(() => first.result.current.run())
    expect(mocks.warning).toHaveBeenCalledWith('Select a connection to run this query.')
    first.unmount()

    const second = renderHook(
      () =>
        useToolbarQueryAction({
          orgSlug: 'acme',
          workspace,
          activeTab: tab,
          hasConnections: false,
        }),
      { wrapper },
    )
    await act(() => second.result.current.run())
    expect(mocks.warning).toHaveBeenCalledWith(
      'No connection available. Add a connection to run queries.',
    )
    second.unmount()
    docs.disposeAll()
  })
})
