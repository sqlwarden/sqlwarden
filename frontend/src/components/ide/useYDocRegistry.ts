import { createContext, useContext } from 'react'
import * as Y from 'yjs'

// ─── Public type ───────────────────────────────────────────────────────────────

export type YDocRegistry = {
  /**
   * Returns the existing Y.Doc for tabId, or creates one.
   * For non-file tabs (scratch, connection) pass initialContent to seed from
   * the persisted IndexedDB snapshot; for file tabs omit it so peer sync or
   * useFileContent owns the content.
   */
  getOrCreate: (tabId: string, initialContent?: string) => Y.Doc
  /** Destroys the doc and closes its BroadcastChannel. No-op if tabId unknown. */
  destroy: (tabId: string) => void
  /** Returns the doc for tabId, or undefined if not yet created. */
  get: (tabId: string) => Y.Doc | undefined
}

// ─── Channel message protocol ──────────────────────────────────────────────────
//
// All messages are plain objects so the type field can be inspected before the
// data is consumed. Uint8Array survives the structured-clone algorithm intact.
//
// Origins on Y.Doc transactions:
//   'init'        — seeded from IndexedDB snapshot; not broadcast, not dirty
//   'server-load' — content replaced from server fetch; broadcast as full-state
//                   so peers can sync; receiver skips if doc non-empty
//   'broadcast'   — applied from a channel message; not re-broadcast, marks dirty
//   undefined     — user typing via yCollab; incremental broadcast, marks dirty

type SyncRequest  = { type: 'sync-request' }
type FullState    = { type: 'full-state';  data: Uint8Array }
type UpdateMsg    = { type: 'update';      data: Uint8Array }
type ChannelMsg   = SyncRequest | FullState | UpdateMsg

// ─── Factory ───────────────────────────────────────────────────────────────────

export function createYDocRegistry(accountId: number): YDocRegistry {
  const entries = new Map<string, { doc: Y.Doc; cleanup: () => void }>()

  function createEntry(tabId: string, initialContent?: string): Y.Doc {
    const doc = new Y.Doc()
    const channel = new BroadcastChannel(`sqlwarden:tab:${accountId}:${tabId}`)

    // ── Outgoing: handle Y.Doc updates ────────────────────────────────────────
    const handleUpdate = (update: Uint8Array, origin: unknown) => {
      if (origin === 'broadcast' || origin === 'init') return

      if (origin === 'server-load') {
        // Broadcast our full state so any peer that is still empty can sync
        // from us rather than re-initialising from server text.
        const full: FullState = { type: 'full-state', data: Y.encodeStateAsUpdate(doc) }
        channel.postMessage(full)
        return
      }

      // User typing (undefined origin) or any other source → incremental update.
      const msg: UpdateMsg = { type: 'update', data: update }
      channel.postMessage(msg)
    }

    // ── Incoming: handle messages from other windows ───────────────────────────
    const handleMessage = (event: MessageEvent<ChannelMsg>) => {
      const msg = event.data
      if (!msg || typeof msg !== 'object' || !('type' in msg)) return

      switch (msg.type) {
        case 'sync-request': {
          // A peer just opened this tab and wants our state.
          const state = Y.encodeStateAsUpdate(doc)
          // Only respond if we have actual content (state > 2 bytes means non-empty).
          if (state.length > 2) {
            const full: FullState = { type: 'full-state', data: state }
            channel.postMessage(full)
          }
          break
        }

        case 'full-state': {
          // Apply only if our doc is still empty — prevents merging two
          // independently-initialised docs with the same text (which would
          // double the content via CRDT union of both insertion sets).
          if (doc.getText('content').length === 0) {
            Y.applyUpdate(doc, new Uint8Array(msg.data), 'broadcast')
          }
          break
        }

        case 'update': {
          // Incremental update from a peer editing the same tab.
          Y.applyUpdate(doc, new Uint8Array(msg.data), 'broadcast')
          break
        }
      }
    }

    doc.on('update', handleUpdate)
    channel.addEventListener('message', handleMessage)

    // Seed content for scratch/connection tabs from the IndexedDB snapshot.
    // File tabs start empty — content arrives via peer sync or useFileContent.
    if (initialContent) {
      doc.transact(() => {
        doc.getText('content').insert(0, initialContent)
      }, 'init')
    }

    // Ask any peer that already has this doc open to share their state.
    // Message delivery is async so the listener above is already active.
    const req: SyncRequest = { type: 'sync-request' }
    channel.postMessage(req)

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
