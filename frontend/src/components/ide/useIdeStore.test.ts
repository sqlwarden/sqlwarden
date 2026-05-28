import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createIdeStore, newScratchTab, newConnectionTab, newFileTab } from './useIdeStore'

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
    store = createIdeStore('test-org')
  })

  it('starts with empty tabs', () => {
    expect(store.getState().tabs).toEqual([])
    expect(store.getState().activeTabId).toBeUndefined()
  })

  it('openTab adds a tab and sets it active', () => {
    const tab = newScratchTab(mockWorkspace)
    store.getState().openTab(tab)
    expect(store.getState().tabs).toHaveLength(1)
    expect(store.getState().activeTabId).toBe(tab.id)
  })

  it('openTab does not duplicate an existing tab id', () => {
    const tab = newConnectionTab(mockConnection, mockWorkspace)
    store.getState().openTab(tab)
    store.getState().openTab(tab)
    expect(store.getState().tabs).toHaveLength(1)
  })

  it('closeTab removes the tab and clears activeTabId when it was active', () => {
    const tab = newScratchTab(mockWorkspace)
    store.getState().openTab(tab)
    store.getState().closeTab(tab.id)
    expect(store.getState().tabs).toHaveLength(0)
    expect(store.getState().activeTabId).toBeUndefined()
  })

  it('closeTab focuses another tab in same workspace when closed tab was active', () => {
    store.getState().setActiveWorkspace(1)
    const tab1 = newScratchTab(mockWorkspace)
    const tab2 = newFileTab(mockFile, mockWorkspace)
    store.getState().openTab(tab1)
    store.getState().openTab(tab2)
    store.getState().closeTab(tab2.id)
    expect(store.getState().activeTabId).toBe(tab1.id)
  })

  it('updateTabContent updates only the target tab', () => {
    const tab = newScratchTab(mockWorkspace)
    store.getState().openTab(tab)
    store.getState().updateTabContent(tab.id, 'SELECT 1;')
    expect(store.getState().tabs[0].content).toBe('SELECT 1;')
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
})
