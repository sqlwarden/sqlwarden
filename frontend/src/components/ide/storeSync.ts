import type { EditorTab, IdeActions, IdeState } from './useIdeStore'

type StoreSyncState = Pick<IdeState, 'sessions' | 'tabs'> &
  Pick<IdeActions, 'clearSession' | 'closeTab' | 'ensureTab' | 'setSession' | 'updateTabEtag'>

export type StoreSyncStore = {
  getState: () => StoreSyncState
  subscribe: (listener: (state: StoreSyncState) => void) => () => void
}

export type StoreSyncChannel = {
  addEventListener: (type: 'message', listener: (event: MessageEvent) => void) => void
  removeEventListener: (type: 'message', listener: (event: MessageEvent) => void) => void
  postMessage: (message: unknown) => void
  close: () => void
}

/** Synchronizes shared IDE tabs, etags, and target sessions between windows. */
export function createStoreSync(store: StoreSyncStore, channel: StoreSyncChannel) {
  const previousEtags = new Map<string, string>()
  const previousTabIds = new Set<string>()
  let previousSessions: Record<number, string> = {}
  let applyingRemote = false
  let seeded = false

  function handleRemote(event: MessageEvent) {
    const message = event.data as Record<string, unknown>
    if (!message?.type) return

    applyingRemote = true
    try {
      if (
        message.type === 'etag-update' &&
        typeof message.tabId === 'string' &&
        typeof message.etag === 'string'
      ) {
        store.getState().updateTabEtag(message.tabId, message.etag)
      } else if (message.type === 'tab-opened' && message.tab) {
        store.getState().ensureTab(message.tab as EditorTab)
      } else if (message.type === 'tab-closed' && typeof message.tabId === 'string') {
        store.getState().closeTab(message.tabId)
      } else if (
        message.type === 'session-set' &&
        typeof message.connectionId === 'number' &&
        typeof message.sessionId === 'string'
      ) {
        store.getState().setSession(message.connectionId, message.sessionId)
      } else if (message.type === 'session-cleared' && typeof message.connectionId === 'number') {
        store.getState().clearSession(message.connectionId)
      }
    } finally {
      applyingRemote = false
    }
  }

  const unsubscribe = store.subscribe((state) => {
    const currentTabIds = new Set(state.tabs.map((tab) => tab.id))

    // Persist hydration is the first store notification. Seed the comparison
    // snapshot without treating restored data as newly-created state.
    if (!seeded) {
      seeded = true
      currentTabIds.forEach((id) => previousTabIds.add(id))
      for (const tab of state.tabs) {
        if (tab.etag !== undefined) previousEtags.set(tab.id, tab.etag)
      }
      previousSessions = { ...state.sessions }
      return
    }

    if (!applyingRemote) {
      for (const tab of state.tabs) {
        const previous = previousEtags.get(tab.id)
        if (tab.etag !== undefined && tab.etag !== previous) {
          channel.postMessage({ type: 'etag-update', tabId: tab.id, etag: tab.etag })
        }
      }
      for (const tab of state.tabs) {
        if (!previousTabIds.has(tab.id) && tab.kind === 'scratch') {
          channel.postMessage({ type: 'tab-opened', tab })
        }
      }
      for (const id of previousTabIds) {
        if (!currentTabIds.has(id) && id.startsWith('scratch:')) {
          channel.postMessage({ type: 'tab-closed', tabId: id })
        }
      }
      for (const [connectionIdText, sessionId] of Object.entries(state.sessions)) {
        const connectionId = Number(connectionIdText)
        if (previousSessions[connectionId] !== sessionId) {
          channel.postMessage({ type: 'session-set', connectionId, sessionId })
        }
      }
      for (const connectionIdText of Object.keys(previousSessions)) {
        const connectionId = Number(connectionIdText)
        if (!(connectionId in state.sessions)) {
          channel.postMessage({ type: 'session-cleared', connectionId })
        }
      }
    }

    previousTabIds.clear()
    currentTabIds.forEach((id) => previousTabIds.add(id))
    previousEtags.clear()
    for (const tab of state.tabs) {
      if (tab.etag !== undefined) previousEtags.set(tab.id, tab.etag)
    }
    previousSessions = { ...state.sessions }
  })

  channel.addEventListener('message', handleRemote)
  return () => {
    unsubscribe()
    channel.removeEventListener('message', handleRemote)
    channel.close()
  }
}
