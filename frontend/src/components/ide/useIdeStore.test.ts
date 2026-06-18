import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createIdeStore, activeTabId, newScratchTab, newConnectionTab, newFileTab, DEFAULT_CONSOLE_CONTENT } from './useIdeStore'

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

describe('useIdeStore', () => {
  let store: ReturnType<typeof createIdeStore>

  beforeEach(() => {
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
    const tab = newScratchTab(mockWorkspace)
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
    const tab = newScratchTab(mockWorkspace)
    store.getState().openTab(tab)
    store.getState().closeTab(tab.id)
    expect(store.getState().tabs).toHaveLength(0)
    expect(active(mockWorkspace.id)).toBeUndefined()
  })

  it('closeTab focuses another tab in same workspace when closed tab was active', () => {
    store.getState().setActiveWorkspace(1)
    const tab1 = newScratchTab(mockWorkspace)
    const tab2 = newFileTab(mockFile, mockWorkspace)
    store.getState().openTab(tab1)
    store.getState().openTab(tab2)
    store.getState().closeTab(tab2.id)
    expect(active(mockWorkspace.id)).toBe(tab1.id)
  })

  it('tabs from different workspaces do not share active tab state', () => {
    const ws2 = { ...mockWorkspace, id: 2, name: 'Analytics' }
    const tab1 = newScratchTab(mockWorkspace)
    const tab2 = newScratchTab(ws2)
    store.getState().openTab(tab1)
    store.getState().openTab(tab2)
    expect(active(mockWorkspace.id)).toBe(tab1.id)
    expect(active(ws2.id)).toBe(tab2.id)
    // switching active workspace doesn't change either workspace's active tab
    store.getState().setActiveWorkspace(mockWorkspace.id)
    expect(active(ws2.id)).toBe(tab2.id)
  })

  it('updateTabContent updates only the target tab', () => {
    const tab = newScratchTab(mockWorkspace)
    store.getState().openTab(tab)
    store.getState().updateTabContent(tab.id, 'SELECT 1;')
    expect(store.getState().tabs[0].content).toBe('SELECT 1;')
  })

  // ── Regression: ySnapshot persistence (reload survival) ────────────────────
  // Before the fix, updateTabContent only stored the text; on page reload the
  // Y.Doc was re-initialised from the stale yState (creation-time empty state)
  // and all console edits were silently discarded.

  it('updateTabContent persists ySnapshot when provided', () => {
    const tab = newScratchTab(mockWorkspace)
    store.getState().openTab(tab)
    const snapshot = [1, 2, 3, 4]
    store.getState().updateTabContent(tab.id, 'SELECT 1;', snapshot)
    expect(store.getState().tabs[0].ySnapshot).toEqual(snapshot)
    expect(store.getState().tabs[0].content).toBe('SELECT 1;')
  })

  it('updateTabContent without ySnapshot does not clear an existing snapshot', () => {
    const tab = newScratchTab(mockWorkspace)
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
    const tab = newScratchTab(mockWorkspace)
    store.getState().openTab(tab)
    const before = active(mockWorkspace.id)
    store.getState().setActiveTab(groupId(mockWorkspace.id), 'nonexistent:id')
    expect(active(mockWorkspace.id)).toBe(before)
  })

  it('setActiveTab updates only the owning workspace entry', () => {
    const ws2 = { ...mockWorkspace, id: 2, name: 'Analytics' }
    const tab1 = newScratchTab(mockWorkspace)
    const tab2a = newScratchTab(ws2)
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
    const tab1 = newScratchTab(mockWorkspace)
    const tab2 = newScratchTab(ws2)
    store.getState().openTab(tab1)
    store.getState().openTab(tab2)
    store.getState().closeTab(tab2.id)
    expect(active(mockWorkspace.id)).toBe(tab1.id)
    expect(active(ws2.id)).toBeUndefined()
  })

  it('setTabConnection persists connectionId on the tab', () => {
    const tab = newScratchTab(mockWorkspace)
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
    const tab = newFileTab(mockFile, mockWorkspace)
    expect(tab.fileId).toBe(20)
    expect(tab.content).toBe('')
    expect(tab.etag).toBeUndefined()
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
    const tab = newScratchTab(mockWorkspace)
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
        store.getState().openTab({ id, workspaceId: mockWorkspace.id, title: id, kind: 'scratch', content: '' })
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
    const t = (id: string) => ({ id, workspaceId: mockWorkspace.id, title: id, kind: 'scratch' as const, content: '' })
    const root = () => store.getState().layout[mockWorkspace.id]

    it('splitActiveTab duplicates the active tab into a new adjacent group', () => {
      store.getState().openTab(t('a'))
      store.getState().openTab(t('b'))
      store.getState().splitActiveTab(mockWorkspace.id, groupId(mockWorkspace.id), 'b', 'right')
      const node = root()
      expect(node.type).toBe('split')
      if (node.type === 'split') {
        // source keeps b; new group also has b (same tab, synced doc)
        expect(node.children.map((c) => (c.type === 'group' ? c.tabIds : []))).toEqual([['a', 'b'], ['b']])
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

describe('activity state', () => {
  it('defaults activeActivityId to "files"', () => {
    const store = createIdeStore('test-org', 1)
    expect(store.getState().activeActivityId).toBe('files')
  })

  it('setActiveActivity switches the active activity', () => {
    const store = createIdeStore('test-org', 1)
    store.getState().setActiveActivity('connections')
    expect(store.getState().activeActivityId).toBe('connections')
  })
})
