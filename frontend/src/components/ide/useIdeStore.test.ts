import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  createIdeStore,
  activeTabId,
  connectionState,
  isNodeExpanded,
  newConnectionTab,
  newFileTab,
  DEFAULT_CONSOLE_CONTENT,
} from './useIdeStore'
import type { EditorTab, IdeState } from './useIdeStore'

vi.mock('idb-keyval', () => ({
  get: vi.fn(() => Promise.resolve(null)),
  set: vi.fn(() => Promise.resolve()),
  del: vi.fn(() => Promise.resolve()),
}))

const mockWorkspace = {
  id: 1,
  name: 'Billing',
  org_id: 1,
  owner_type: 'org' as const,
  owner_id: 1,
  environment_count: 0,
  connection_count: 0,
  created_at: '',
  updated_at: '',
}
const mockConnection = {
  id: 10,
  workspace_id: 1,
  environment_id: 5,
  name: 'billing-pg',
  driver: 'postgres',
  access_mode: 'open' as const,
  created_at: '',
  updated_at: '',
}
const mockFile = {
  id: 20,
  workspace_id: 1,
  visibility: 'private' as const,
  object_type: 'file' as const,
  name: 'query.sql',
  created_by: 1,
  updated_by: 1,
  created_at: '',
  updated_at: '',
}

let scratchFixtureId = 0

function scratchTabFixture(workspace: typeof mockWorkspace): EditorTab {
  return {
    id: `scratch:${workspace.id}:fixture-${scratchFixtureId++}`,
    workspaceId: workspace.id,
    title: 'Console',
    kind: 'scratch',
    content: DEFAULT_CONSOLE_CONTENT,
  }
}

describe('useIdeStore', () => {
  let store: ReturnType<typeof createIdeStore>

  beforeEach(() => {
    scratchFixtureId = 0
    store = createIdeStore('test-org', 1)
  })

  // The focused group's active tab id for a workspace (replaces the old activeTabIds map).
  const active = (ws: number) => activeTabId(store.getState(), ws)
  // The focused group id for a workspace (single-group in most tests).
  const groupId = (ws: number) => store.getState().activeGroupId[ws]

  it('starts with empty tabs', () => {
    expect(store.getState().tabs).toEqual([])
    expect(store.getState().layout).toEqual({})
  })

  it('openTab adds a tab and sets it active for its workspace', () => {
    const tab = scratchTabFixture(mockWorkspace)
    store.getState().openTab(tab)
    expect(store.getState().tabs).toHaveLength(1)
    expect(active(mockWorkspace.id)).toBe(tab.id)
  })

  it('openTab does not duplicate an existing tab id', () => {
    const tab = newConnectionTab(mockConnection, mockWorkspace)
    store.getState().openTab(tab)
    store.getState().openTab(tab)
    expect(store.getState().tabs).toHaveLength(1)
  })

  it('closeTab removes the tab and clears activeTabId for its workspace', () => {
    const tab = scratchTabFixture(mockWorkspace)
    store.getState().openTab(tab)
    store.getState().closeTab(tab.id)
    expect(store.getState().tabs).toHaveLength(0)
    expect(active(mockWorkspace.id)).toBeUndefined()
  })

  it('closeTab focuses another tab in same workspace when closed tab was active', () => {
    store.getState().setActiveWorkspace(1)
    const tab1 = scratchTabFixture(mockWorkspace)
    const tab2 = newFileTab(mockFile, mockWorkspace)
    store.getState().openTab(tab1)
    store.getState().openTab(tab2)
    store.getState().closeTab(tab2.id)
    expect(active(mockWorkspace.id)).toBe(tab1.id)
  })

  it('tabs from different workspaces do not share active tab state', () => {
    const ws2 = { ...mockWorkspace, id: 2, name: 'Analytics' }
    const tab1 = scratchTabFixture(mockWorkspace)
    const tab2 = scratchTabFixture(ws2)
    store.getState().openTab(tab1)
    store.getState().openTab(tab2)
    expect(active(mockWorkspace.id)).toBe(tab1.id)
    expect(active(ws2.id)).toBe(tab2.id)
    // switching active workspace doesn't change either workspace's active tab
    store.getState().setActiveWorkspace(mockWorkspace.id)
    expect(active(ws2.id)).toBe(tab2.id)
  })

  it('updateTabContent updates only the target tab', () => {
    const tab = scratchTabFixture(mockWorkspace)
    store.getState().openTab(tab)
    store.getState().updateTabContent(tab.id, 'SELECT 1;')
    expect(store.getState().tabs[0].content).toBe('SELECT 1;')
  })

  it('renames a file tab without changing its editor state', () => {
    const tab = newFileTab(mockFile, mockWorkspace)
    store.getState().openTab(tab)
    store.getState().updateTabEtag(tab.id, 'etag-1')
    store.getState().updateTabContent(tab.id, 'select 1', [1, 2, 3])
    const before = store.getState().tabs[0]

    store.getState().renameTabByFileId(mockFile.id, 'renamed.sql')

    const after = store.getState().tabs[0]
    expect(after).toEqual({ ...before, title: 'renamed.sql', subtitle: 'renamed.sql' })
    expect(after.id).toBe(tab.id)
  })

  // ── Regression: ySnapshot persistence (reload survival) ────────────────────
  // Before the fix, updateTabContent only stored the text; on page reload the
  // Y.Doc was re-initialised from the stale yState (creation-time empty state)
  // and all console edits were silently discarded.

  it('updateTabContent persists ySnapshot when provided', () => {
    const tab = scratchTabFixture(mockWorkspace)
    store.getState().openTab(tab)
    const snapshot = [1, 2, 3, 4]
    store.getState().updateTabContent(tab.id, 'SELECT 1;', snapshot)
    expect(store.getState().tabs[0].ySnapshot).toEqual(snapshot)
    expect(store.getState().tabs[0].content).toBe('SELECT 1;')
  })

  it('updateTabContent without ySnapshot does not clear an existing snapshot', () => {
    const tab = scratchTabFixture(mockWorkspace)
    store.getState().openTab(tab)
    store.getState().updateTabContent(tab.id, 'SELECT 1;', [1, 2, 3])
    store.getState().updateTabContent(tab.id, 'SELECT 2;')
    // snapshot from first write must survive the second write
    expect(store.getState().tabs[0].ySnapshot).toEqual([1, 2, 3])
    expect(store.getState().tabs[0].content).toBe('SELECT 2;')
  })

  it('updateTabContent with ySnapshot marks file tab dirty', () => {
    const tab = newFileTab(mockFile, mockWorkspace)
    store.getState().openTab(tab)
    store.getState().updateTabEtag(tab.id, 'etag-1')
    store.getState().updateTabContent(tab.id, 'SELECT 99;', [5, 6, 7])
    expect(store.getState().tabs[0].isDirty).toBe(true)
    expect(store.getState().tabs[0].ySnapshot).toEqual([5, 6, 7])
  })

  // ── Regression: per-workspace active tab isolation ─────────────────────────
  // Before the fix, a single activeTabId was shared across all workspaces.
  // Switching to workspace 2 left workspace 1's tab rendered in the editor.

  it('setActiveTab is a no-op for an unknown tabId', () => {
    const tab = scratchTabFixture(mockWorkspace)
    store.getState().openTab(tab)
    const before = active(mockWorkspace.id)
    store.getState().setActiveTab(groupId(mockWorkspace.id), 'nonexistent:id')
    expect(active(mockWorkspace.id)).toBe(before)
  })

  it('setActiveTab updates only the owning workspace entry', () => {
    const ws2 = { ...mockWorkspace, id: 2, name: 'Analytics' }
    const tab1 = scratchTabFixture(mockWorkspace)
    const tab2a = scratchTabFixture(ws2)
    const tab2b = newFileTab(mockFile, ws2)
    store.getState().openTab(tab1)
    store.getState().openTab(tab2a)
    store.getState().openTab(tab2b)
    store.getState().setActiveTab(groupId(ws2.id), tab2b.id)
    // ws2 switched to tab2b
    expect(active(ws2.id)).toBe(tab2b.id)
    // ws1 is unaffected
    expect(active(mockWorkspace.id)).toBe(tab1.id)
  })

  it('closeTab on the active tab of workspace 2 does not change workspace 1 active tab', () => {
    const ws2 = { ...mockWorkspace, id: 2, name: 'Analytics' }
    const tab1 = scratchTabFixture(mockWorkspace)
    const tab2 = scratchTabFixture(ws2)
    store.getState().openTab(tab1)
    store.getState().openTab(tab2)
    store.getState().closeTab(tab2.id)
    expect(active(mockWorkspace.id)).toBe(tab1.id)
    expect(active(ws2.id)).toBeUndefined()
  })

  it('setTabConnection persists connectionId on the tab', () => {
    const tab = scratchTabFixture(mockWorkspace)
    store.getState().openTab(tab)
    store.getState().setTabConnection(tab.id, 42)
    expect(store.getState().tabs[0].connectionId).toBe(42)
  })

  it('newConnectionTab uses connection id as tab id', () => {
    const tab = newConnectionTab(mockConnection, mockWorkspace)
    expect(tab.id).toBe('connection:10')
    expect(tab.connectionId).toBe(10)
    expect(tab.kind).toBe('connection')
  })

  it('newFileTab uses file id as tab id', () => {
    const tab = newFileTab(mockFile, mockWorkspace)
    expect(tab.id).toBe('file:20')
    expect(tab.kind).toBe('file')
  })

  it('newFileTab includes fileId and empty content', () => {
    const tab = newFileTab(
      {
        ...mockFile,
        media_type: 'text/csv; charset=utf-8',
        file_kind: 'export',
        size_bytes: 2048,
      },
      mockWorkspace,
    )
    expect(tab.fileId).toBe(20)
    expect(tab.content).toBe('')
    expect(tab.etag).toBeUndefined()
    expect(tab.fileMediaType).toBe('text/csv; charset=utf-8')
    expect(tab.fileKind).toBe('export')
    expect(tab.fileSizeBytes).toBe(2048)
  })

  it('updateTabEtag stores etag and sets isDirty false', () => {
    const tab = newFileTab(mockFile, mockWorkspace)
    store.getState().openTab(tab)
    store.getState().updateTabEtag(tab.id, 'abc123')
    expect(store.getState().tabs[0].etag).toBe('abc123')
    expect(store.getState().tabs[0].isDirty).toBe(false)
  })

  it('updateTabContent marks file tab dirty after etag is set', () => {
    const tab = newFileTab(mockFile, mockWorkspace)
    store.getState().openTab(tab)
    store.getState().updateTabEtag(tab.id, 'abc123')
    store.getState().updateTabContent(tab.id, 'SELECT 2;')
    expect(store.getState().tabs[0].isDirty).toBe(true)
  })

  it('updateTabContent does not mark scratch tab dirty', () => {
    const tab = scratchTabFixture(mockWorkspace)
    store.getState().openTab(tab)
    store.getState().updateTabContent(tab.id, 'SELECT 2;')
    expect(store.getState().tabs[0].isDirty).toBeUndefined()
  })

  it('updateTabEtag clears isDirty after content was edited', () => {
    const tab = newFileTab(mockFile, mockWorkspace)
    store.getState().openTab(tab)
    store.getState().updateTabEtag(tab.id, 'abc123')
    store.getState().updateTabContent(tab.id, 'SELECT 2;')
    store.getState().updateTabEtag(tab.id, 'def456')
    expect(store.getState().tabs[0].isDirty).toBe(false)
    expect(store.getState().tabs[0].etag).toBe('def456')
  })

  // ── Console close-warning predicate ───────────────────────────────────────
  // IdeTabBar shows a confirmation dialog before closing a scratch tab that
  // has content. These tests verify the store state the predicate reads from.

  it('scratch tab starts with empty DEFAULT_CONSOLE_CONTENT so no warning is shown for new consoles', () => {
    store.getState().openConsole(mockWorkspace, [])
    // IdeTabBar condition: tab.kind === 'scratch' && tab.content.trim() !== ''
    // A freshly opened console must NOT trigger the warning.
    expect(DEFAULT_CONSOLE_CONTENT.trim()).toBe('')
    expect(store.getState().tabs[0].content.trim()).toBe('')
  })

  it('scratch tab has non-empty content after user edits, triggering the close warning', () => {
    store.getState().openConsole(mockWorkspace, [])
    const tab = store.getState().tabs[0]
    store.getState().updateTabContent(tab.id, 'SELECT 1;')
    // IdeTabBar condition: tab.content.trim() !== '' → show confirmation dialog
    expect(store.getState().tabs[0].content.trim()).not.toBe('')
  })

  it('openConsole creates a numbered scratch tab with yState', () => {
    const fakeYState = [1, 2, 3]
    store.getState().openConsole(mockWorkspace, fakeYState)
    const tabs = store.getState().tabs
    expect(tabs).toHaveLength(1)
    expect(tabs[0].id).toBe('scratch:1:1')
    expect(tabs[0].title).toBe('Console 1')
    expect(tabs[0].kind).toBe('scratch')
    expect(tabs[0].content).toBe(DEFAULT_CONSOLE_CONTENT)
    expect(tabs[0].yState).toEqual(fakeYState)
  })

  it('openConsole increments counter on each call', () => {
    store.getState().openConsole(mockWorkspace, [1])
    store.getState().openConsole(mockWorkspace, [2])
    const tabs = store.getState().tabs
    expect(tabs[0].id).toBe('scratch:1:1')
    expect(tabs[1].id).toBe('scratch:1:2')
    expect(tabs[1].title).toBe('Console 2')
  })

  it('openConsole numbers from highest+1 after closing tabs', () => {
    store.getState().openConsole(mockWorkspace, [1])
    store.getState().openConsole(mockWorkspace, [2])
    store.getState().closeTab('scratch:1:1')
    store.getState().closeTab('scratch:1:2')
    store.getState().openConsole(mockWorkspace, [3])
    expect(store.getState().tabs[0].id).toBe('scratch:1:1')
    expect(store.getState().tabs[0].title).toBe('Console 1')
  })

  it('openConsole sets the new tab as active for its workspace', () => {
    store.getState().openConsole(mockWorkspace, [])
    expect(active(mockWorkspace.id)).toBe('scratch:1:1')
  })

  describe('moveTab', () => {
    function addTabs(ids: string[]) {
      for (const id of ids) {
        store
          .getState()
          .openTab({ id, workspaceId: mockWorkspace.id, title: id, kind: 'scratch', content: '' })
      }
    }
    const order = () => {
      const node = store.getState().layout[mockWorkspace.id]
      return node && node.type === 'group' ? node.tabIds : []
    }

    const g = () => groupId(mockWorkspace.id)

    it('moves a tab before a target', () => {
      addTabs(['a', 'b', 'c', 'd'])
      store.getState().moveTab(g(), 'a', g(), 'c', 'before')
      expect(order()).toEqual(['b', 'a', 'c', 'd'])
    })

    it('moves a tab after a target, supporting move-to-end', () => {
      addTabs(['a', 'b', 'c', 'd'])
      store.getState().moveTab(g(), 'a', g(), 'd', 'after')
      expect(order()).toEqual(['b', 'c', 'd', 'a'])
    })

    it('moves a later tab before an earlier one', () => {
      addTabs(['a', 'b', 'c', 'd'])
      store.getState().moveTab(g(), 'd', g(), 'a', 'before')
      expect(order()).toEqual(['d', 'a', 'b', 'c'])
    })

    it('is a no-op when dragged equals target', () => {
      addTabs(['a', 'b', 'c'])
      store.getState().moveTab(g(), 'b', g(), 'b', 'before')
      expect(order()).toEqual(['a', 'b', 'c'])
    })

    it('does not churn the layout on repeated identical moves', () => {
      addTabs(['a', 'b', 'c'])
      store.getState().moveTab(g(), 'a', g(), 'b', 'before') // a now active, order [a,b,c]
      const afterFirst = store.getState().layout[mockWorkspace.id]
      store.getState().moveTab(g(), 'a', g(), 'b', 'before') // identical → no-op (avoids dragover churn)
      expect(store.getState().layout[mockWorkspace.id]).toBe(afterFirst)
    })
  })

  describe('layout actions', () => {
    const t = (id: string) => ({
      id,
      workspaceId: mockWorkspace.id,
      title: id,
      kind: 'scratch' as const,
      content: '',
    })
    const root = () => store.getState().layout[mockWorkspace.id]

    it('splitActiveTab duplicates the active tab into a new adjacent group', () => {
      store.getState().openTab(t('a'))
      store.getState().openTab(t('b'))
      store.getState().splitActiveTab(mockWorkspace.id, groupId(mockWorkspace.id), 'b', 'right')
      const node = root()
      expect(node.type).toBe('split')
      if (node.type === 'split') {
        // source keeps b; new group also has b (same tab, synced doc)
        expect(node.children.map((c) => (c.type === 'group' ? c.tabIds : []))).toEqual([
          ['a', 'b'],
          ['b'],
        ])
      }
    })

    it('closeTabInstance closes one pane but keeps the tab while another pane shows it', () => {
      store.getState().openTab(t('a'))
      store.getState().splitActiveTab(mockWorkspace.id, groupId(mockWorkspace.id), 'a', 'right') // a now in two groups
      const newGroupId = store.getState().activeGroupId[mockWorkspace.id]
      store.getState().closeTabInstance(newGroupId, 'a')
      expect(store.getState().tabs.some((tab) => tab.id === 'a')).toBe(true) // still open in the other pane
      expect(root().type).toBe('group')
    })

    it('closeTabInstance fully closes the tab on its last instance', () => {
      store.getState().openTab(t('a'))
      const gid = store.getState().activeGroupId[mockWorkspace.id]
      store.getState().closeTabInstance(gid, 'a')
      expect(store.getState().tabs.some((tab) => tab.id === 'a')).toBe(false)
    })

    it('closeTab fully closes a tab across all panes', () => {
      store.getState().openTab(t('a'))
      store.getState().splitActiveTab(mockWorkspace.id, groupId(mockWorkspace.id), 'a', 'right') // a in two groups
      store.getState().closeTab('a')
      expect(store.getState().tabs.some((tab) => tab.id === 'a')).toBe(false)
      expect(root().type).toBe('group') // collapsed back to one group
    })
  })
})

describe('connectionState', () => {
  const base = (over: Partial<IdeState>): IdeState =>
    ({ sessions: {}, connectionStatus: {}, ...over }) as unknown as IdeState

  it('reports connected when a live session exists (even over a prior error)', () => {
    expect(
      connectionState(base({ sessions: { 5: 'sid' }, connectionStatus: { 5: { error: 'x' } } }), 5),
    ).toEqual({ kind: 'connected' })
  })
  it('reports connecting', () => {
    expect(connectionState(base({ connectionStatus: { 5: 'connecting' } }), 5)).toEqual({
      kind: 'connecting',
    })
  })
  it('reports error with its message', () => {
    expect(connectionState(base({ connectionStatus: { 5: { error: 'auth failed' } } }), 5)).toEqual(
      { kind: 'error', message: 'auth failed' },
    )
  })
  it('reports idle when nothing is known', () => {
    expect(connectionState(base({}), 5)).toEqual({ kind: 'idle' })
  })
})

describe('isNodeExpanded', () => {
  it('returns the stored value when present', () => {
    expect(isNodeExpanded({ 'conn:1': true }, 'conn:1', false)).toBe(true)
    expect(isNodeExpanded({ 'conn:1': false }, 'conn:1', true)).toBe(false)
  })
  it('returns the fallback when absent', () => {
    expect(isNodeExpanded({}, 'conn:1', true)).toBe(true)
    expect(isNodeExpanded({}, 'conn:1', false)).toBe(false)
  })
})

describe('expansion actions', () => {
  it('setNodeExpanded sets a key and collapseAllNodes clears the map', () => {
    const store = createIdeStore('test-org', 1)
    store.getState().setNodeExpanded('conn:1', true)
    store.getState().setNodeExpanded('env:2', true)
    expect(store.getState().expandedNodes).toEqual({ 'conn:1': true, 'env:2': true })
    store.getState().collapseAllNodes()
    expect(store.getState().expandedNodes).toEqual({})
  })
})

describe('activity state', () => {
  it('defaults activeActivityId to "connections"', () => {
    const store = createIdeStore('test-org', 1)
    expect(store.getState().activeActivityId).toBe('connections')
  })

  it('setActiveActivity switches the active activity', () => {
    const store = createIdeStore('test-org', 1)
    store.getState().setActiveActivity('connections')
    expect(store.getState().activeActivityId).toBe('connections')
  })
})

describe('pendingJump', () => {
  it('setPendingJump stores a jump target and clearPendingJump resets it', () => {
    const store = createIdeStore('test-org', 1)
    expect(store.getState().pendingJump).toBeNull()
    store.getState().setPendingJump({ tabId: 'file:9', line: 2, column: 4 })
    expect(store.getState().pendingJump).toEqual({ tabId: 'file:9', line: 2, column: 4 })
    store.getState().clearPendingJump()
    expect(store.getState().pendingJump).toBeNull()
  })
})

describe('syncSessions', () => {
  it('replaces the whole map when no scope is given', () => {
    const store = createIdeStore('test-org', 1)
    store.getState().setSession(1, 'a')
    store.getState().setSession(2, 'b')
    store.getState().syncSessions({ 3: 'c' })
    expect(store.getState().sessions).toEqual({ 3: 'c' })
  })

  it('scoped sync only touches the given connections', () => {
    const store = createIdeStore('test-org', 1)
    // Connection 9 belongs to another workspace and must survive a scoped sync.
    store.getState().setSession(9, 'other-ws')
    store.getState().setSession(1, 'stale')
    store.getState().setSession(2, 'still-alive')
    store.getState().syncSessions({ 2: 'still-alive' }, [1, 2, 3])
    expect(store.getState().sessions).toEqual({ 9: 'other-ws', 2: 'still-alive' })
  })
})

describe('query results', () => {
  it('beginRun creates a run with one pending entry per statement and selects it', () => {
    const store = createIdeStore('test-org', 1)
    const runId = store.getState().beginRun('tab-1', ['select 1', 'select 2'])
    const runs = store.getState().resultRuns['tab-1']
    expect(runs).toHaveLength(1)
    expect(runs[0]).toEqual(
      expect.objectContaining({
        id: runId,
        results: [
          { status: 'pending', sql: 'select 1' },
          { status: 'pending', sql: 'select 2' },
        ],
        selectedIndex: 0,
      }),
    )
    expect(store.getState().selectedRunId['tab-1']).toBe(runId)
  })

  it('beginRun stores the connection id the run started with', () => {
    const store = createIdeStore('test-org', 1)
    const runId = store.getState().beginRun('tab-1', ['select 1'], 7)
    const runs = store.getState().resultRuns['tab-1']
    expect(runs[0]).toEqual(expect.objectContaining({ id: runId, connectionId: 7 }))
  })

  it('beginRun appends a new run without touching earlier runs', () => {
    const store = createIdeStore('test-org', 1)
    const first = store.getState().beginRun('tab-1', ['select 1'])
    const second = store.getState().beginRun('tab-1', ['select 2'])
    const runs = store.getState().resultRuns['tab-1']
    expect(runs.map((r) => r.id)).toEqual([first, second])
    expect(store.getState().selectedRunId['tab-1']).toBe(second)
  })

  it('setRunStatementResult updates one entry within the matching run only', () => {
    const store = createIdeStore('test-org', 1)
    const runId = store.getState().beginRun('tab-1', ['select 1', 'select 2'])
    store
      .getState()
      .setRunStatementResult('tab-1', runId, 1, { status: 'running', sql: 'select 2' })
    expect(store.getState().resultRuns['tab-1'][0].results).toEqual([
      { status: 'pending', sql: 'select 1' },
      { status: 'running', sql: 'select 2' },
    ])
  })

  it('markRunRemainingSkipped marks every non-terminal entry in the run from fromIndex onward', () => {
    const store = createIdeStore('test-org', 1)
    const runId = store.getState().beginRun('tab-1', ['select 1', 'select 2', 'select 3'])
    store.getState().setRunStatementResult('tab-1', runId, 0, {
      status: 'ok',
      durationMs: 1,
      sql: 'select 1',
      data: {
        columns: [],
        rows: [],
        duration_ms: 1,
        truncated: false,
        rows_returned: 0,
        bytes_returned: 0,
      },
    })
    store.getState().markRunRemainingSkipped('tab-1', runId, 1)
    expect(store.getState().resultRuns['tab-1'][0].results).toEqual([
      expect.objectContaining({ status: 'ok' }),
      { status: 'skipped', sql: 'select 2' },
      { status: 'skipped', sql: 'select 3' },
    ])
  })

  it('evictRuns removes the given runs and leaves the rest untouched', () => {
    const store = createIdeStore('test-org', 1)
    const first = store.getState().beginRun('tab-1', ['select 1'])
    const second = store.getState().beginRun('tab-1', ['select 2'])
    store.getState().evictRuns('tab-1', [first])
    expect(store.getState().resultRuns['tab-1'].map((r) => r.id)).toEqual([second])
  })

  it('setSelectedRun changes which run is active for a tab', () => {
    const store = createIdeStore('test-org', 1)
    const first = store.getState().beginRun('tab-1', ['select 1'])
    store.getState().beginRun('tab-1', ['select 2'])
    store.getState().setSelectedRun('tab-1', first)
    expect(store.getState().selectedRunId['tab-1']).toBe(first)
  })

  it('setSelectedIndexInRun updates the selected statement index within one run', () => {
    const store = createIdeStore('test-org', 1)
    const runId = store.getState().beginRun('tab-1', ['select 1', 'select 2'])
    store.getState().setSelectedIndexInRun('tab-1', runId, 1)
    expect(store.getState().resultRuns['tab-1'][0].selectedIndex).toBe(1)
  })

  it('closeRunTab removes one run and reselects the previous run', () => {
    const store = createIdeStore('test-org', 1)
    const first = store.getState().beginRun('tab-1', ['select 1'])
    const second = store.getState().beginRun('tab-1', ['select 2'])
    store.getState().closeRunTab('tab-1', second)
    expect(store.getState().resultRuns['tab-1'].map((r) => r.id)).toEqual([first])
    expect(store.getState().selectedRunId['tab-1']).toBe(first)
  })

  it('closeRunTab falls back to the next remaining run when the first run closes', () => {
    const store = createIdeStore('test-org', 1)
    const first = store.getState().beginRun('tab-1', ['select 1'])
    const second = store.getState().beginRun('tab-1', ['select 2'])
    store.getState().setSelectedRun('tab-1', first)
    store.getState().closeRunTab('tab-1', first)
    expect(store.getState().resultRuns['tab-1'].map((r) => r.id)).toEqual([second])
    expect(store.getState().selectedRunId['tab-1']).toBe(second)
  })

  it('abandonPendingRunConfirmation clears the confirmation and skips from its statementIndex', () => {
    const store = createIdeStore('test-org', 1)
    const runId = store.getState().beginRun('tab-1', ['select 1', 'select 2', 'select 3'])
    store.getState().setPendingConfirmation('tab-1', {
      sql: 'select 2',
      statements: [],
      runId,
      statementIndex: 1,
    })
    store.getState().abandonPendingRunConfirmation('tab-1')
    expect(store.getState().pendingConfirmations['tab-1']).toBeUndefined()
    expect(store.getState().resultRuns['tab-1'][0].results).toEqual([
      { status: 'pending', sql: 'select 1' },
      { status: 'skipped', sql: 'select 2' },
      { status: 'skipped', sql: 'select 3' },
    ])
  })

  it('closeTab clears the run history and selected run for the closed tab', () => {
    const store = createIdeStore('test-org', 1)
    store.getState().beginRun('tab-1', ['select 1'])
    store.getState().closeTab('tab-1')
    expect(store.getState().resultRuns['tab-1']).toBeUndefined()
    expect(store.getState().selectedRunId['tab-1']).toBeUndefined()
  })
})
