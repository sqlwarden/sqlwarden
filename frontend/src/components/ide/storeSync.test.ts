import { createStore } from 'zustand/vanilla'
import { describe, expect, it, vi } from 'vitest'
import { createStoreSync, type StoreSyncChannel, type StoreSyncStore } from './storeSync'
import type { EditorTab } from './useIdeStore'

type TestState = ReturnType<StoreSyncStore['getState']>

function testStore() {
  return createStore<TestState>((set) => ({
    tabs: [],
    sessions: {},
    ensureTab: (tab) => set((state) => state.tabs.some((item) => item.id === tab.id)
      ? state
      : { ...state, tabs: [...state.tabs, tab] }),
    closeTab: (tabId) => set((state) => ({ ...state, tabs: state.tabs.filter((tab) => tab.id !== tabId) })),
    updateTabEtag: (tabId, etag) => set((state) => ({
      ...state,
      tabs: state.tabs.map((tab) => tab.id === tabId ? { ...tab, etag } : tab),
    })),
    setSession: (connectionId, sessionId) => set((state) => ({
      ...state,
      sessions: { ...state.sessions, [connectionId]: sessionId },
    })),
    clearSession: (connectionId) => set((state) => {
      const sessions = { ...state.sessions }
      delete sessions[connectionId]
      return { ...state, sessions }
    }),
  }))
}

function testChannel() {
  let listener: ((event: MessageEvent) => void) | undefined
  const channel: StoreSyncChannel & { receive: (data: unknown) => void } = {
    addEventListener: vi.fn((_type, next) => { listener = next }),
    removeEventListener: vi.fn(),
    postMessage: vi.fn(),
    close: vi.fn(),
    receive: (data) => listener?.(new MessageEvent('message', { data })),
  }
  return channel
}

const scratch: EditorTab = {
  id: 'scratch:1:1',
  workspaceId: 1,
  title: 'Console 1',
  kind: 'scratch',
  content: '',
}

describe('createStoreSync', () => {
  it('seeds without broadcasting restored state, then broadcasts local changes', () => {
    const store = testStore()
    const channel = testChannel()
    createStoreSync(store, channel)

    store.setState((state) => ({ ...state }))
    expect(channel.postMessage).not.toHaveBeenCalled()

    store.getState().ensureTab(scratch)
    store.getState().setSession(7, 'session-7')
    expect(channel.postMessage).toHaveBeenCalledWith({ type: 'tab-opened', tab: scratch })
    expect(channel.postMessage).toHaveBeenCalledWith({ type: 'session-set', connectionId: 7, sessionId: 'session-7' })
  })

  it('applies remote changes without echoing them', () => {
    const store = testStore()
    const channel = testChannel()
    createStoreSync(store, channel)
    store.setState((state) => ({ ...state }))

    channel.receive({ type: 'tab-opened', tab: scratch })
    channel.receive({ type: 'session-set', connectionId: 7, sessionId: 'remote' })

    expect(store.getState().tabs).toContainEqual(scratch)
    expect(store.getState().sessions[7]).toBe('remote')
    expect(channel.postMessage).not.toHaveBeenCalled()
  })

  it('stops listening and closes the channel during cleanup', () => {
    const store = testStore()
    const channel = testChannel()
    const cleanup = createStoreSync(store, channel)
    cleanup()
    expect(channel.removeEventListener).toHaveBeenCalledOnce()
    expect(channel.close).toHaveBeenCalledOnce()
  })
})
