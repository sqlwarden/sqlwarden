import { createContext, useContext } from 'react'
import * as Y from 'yjs'

// ─── Public type ───────────────────────────────────────────────────────────────

export type YDocRegistry = {
  /**
   * Returns the existing Y.Doc for tabId, or creates one.
   * When created, initialContent (if provided) is inserted with origin 'init'
   * so it is not broadcast to other windows.
   */
  getOrCreate: (tabId: string, initialContent?: string) => Y.Doc
  /** Destroys the doc and closes its BroadcastChannel. No-op if tabId unknown. */
  destroy: (tabId: string) => void
  /** Returns the doc for tabId, or undefined if not yet created. */
  get: (tabId: string) => Y.Doc | undefined
}

// ─── Factory ───────────────────────────────────────────────────────────────────

export function createYDocRegistry(): YDocRegistry {
  const entries = new Map<string, { doc: Y.Doc; cleanup: () => void }>()

  function createEntry(tabId: string, initialContent?: string): Y.Doc {
    const doc = new Y.Doc()

    // Channel name is per-tabId: same file opened in two windows shares the channel
    // (file tabs use id `file:{fileId}`); scratch tabs have unique timestamp ids
    // so they never cross-sync.
    const channel = new BroadcastChannel(`sqlwarden:tab:${tabId}`)

    // Send user-initiated updates to other windows.
    // 'broadcast', 'server-load', and 'init' origins are excluded:
    //   broadcast   — prevents echo loops
    //   server-load — server fetch must not propagate; other windows fetch independently
    //   init        — initial seed from store snapshot; not a user edit
    const handleUpdate = (update: Uint8Array, origin: unknown) => {
      if (origin === 'broadcast' || origin === 'server-load' || origin === 'init') return
      channel.postMessage(update)
    }

    // Apply updates received from other windows with origin 'broadcast' so the
    // handleUpdate guard above prevents re-broadcasting them.
    const handleMessage = (event: MessageEvent<ArrayBuffer>) => {
      Y.applyUpdate(doc, new Uint8Array(event.data), 'broadcast')
    }

    doc.on('update', handleUpdate)
    channel.addEventListener('message', handleMessage)

    // Seed initial content without broadcasting.
    if (initialContent) {
      doc.transact(() => {
        doc.getText('content').insert(0, initialContent)
      }, 'init')
    }

    entries.set(tabId, {
      doc,
      cleanup: () => {
        doc.off('update', handleUpdate)
        channel.removeEventListener('message', handleMessage)
        channel.close()
        doc.destroy()
      },
    })

    return doc
  }

  return {
    getOrCreate(tabId, initialContent) {
      return entries.get(tabId)?.doc ?? createEntry(tabId, initialContent)
    },
    destroy(tabId) {
      const entry = entries.get(tabId)
      if (!entry) return
      entry.cleanup()
      entries.delete(tabId)
    },
    get(tabId) {
      return entries.get(tabId)?.doc
    },
  }
}

// ─── React context ─────────────────────────────────────────────────────────────

export const YDocRegistryContext = createContext<YDocRegistry | null>(null)

export function useYDocRegistry(): YDocRegistry {
  const ctx = useContext(YDocRegistryContext)
  if (!ctx) throw new Error('useYDocRegistry must be used within a YDocRegistryContext.Provider')
  return ctx
}
