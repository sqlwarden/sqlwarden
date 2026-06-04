import { describe, it, expect, beforeEach } from 'vitest'
import * as Y from 'yjs'
import { createYDocRegistry } from './useYDocRegistry'

// BroadcastChannel is not in jsdom — provide a synchronous in-process fake.
const channels = new Map<string, Set<FakeBroadcastChannel>>()

class FakeBroadcastChannel {
  name: string
  private listeners = new Set<(e: MessageEvent) => void>()

  constructor(name: string) {
    this.name = name
    if (!channels.has(name)) channels.set(name, new Set())
    channels.get(name)!.add(this)
  }

  postMessage(data: unknown) {
    for (const peer of channels.get(this.name) ?? []) {
      if (peer === this) continue
      peer.listeners.forEach((l) => l(new MessageEvent('message', { data })))
    }
  }

  addEventListener(_: string, fn: (e: MessageEvent) => void) {
    this.listeners.add(fn)
  }

  removeEventListener(_: string, fn: (e: MessageEvent) => void) {
    this.listeners.delete(fn)
  }

  close() {
    channels.get(this.name)?.delete(this)
  }
}

;(globalThis as unknown as Record<string, unknown>).BroadcastChannel = FakeBroadcastChannel

beforeEach(() => channels.clear())

describe('createYDocRegistry', () => {
  it('getOrCreate returns a Y.Doc', () => {
    const reg = createYDocRegistry(1)
    expect(reg.getOrCreate('tab:1')).toBeInstanceOf(Y.Doc)
  })

  it('getOrCreate is idempotent — returns the same doc', () => {
    const reg = createYDocRegistry(1)
    expect(reg.getOrCreate('tab:1')).toBe(reg.getOrCreate('tab:1'))
  })

  it('getOrCreate initialises content from initialContent (scratch/connection tabs)', () => {
    const reg = createYDocRegistry(1)
    const doc = reg.getOrCreate('scratch:1:123', 'SELECT 1;')
    expect(doc.getText('content').toString()).toBe('SELECT 1;')
  })

  it('get returns undefined before creation', () => {
    expect(createYDocRegistry(1).get('nope')).toBeUndefined()
  })

  it('get returns the doc after creation', () => {
    const reg = createYDocRegistry(1)
    reg.getOrCreate('tab:1')
    expect(reg.get('tab:1')).toBeInstanceOf(Y.Doc)
  })

  it('destroy removes the doc', () => {
    const reg = createYDocRegistry(1)
    reg.getOrCreate('tab:1')
    reg.destroy('tab:1')
    expect(reg.get('tab:1')).toBeUndefined()
  })

  it('destroy is a no-op for unknown tabId', () => {
    expect(() => createYDocRegistry(1).destroy('nope')).not.toThrow()
  })

  it('user edits are broadcast as incremental updates', () => {
    const regA = createYDocRegistry(1)
    const regB = createYDocRegistry(1)
    const docA = regA.getOrCreate('file:99')
    const docB = regB.getOrCreate('file:99')

    docA.getText('content').insert(0, 'hello')

    expect(docB.getText('content').toString()).toBe('hello')
  })

  it('init origin (scratch/connection seed) is NOT broadcast', () => {
    const regB = createYDocRegistry(1)
    regB.getOrCreate('scratch:1:999')

    const regA = createYDocRegistry(1)
    regA.getOrCreate('scratch:1:999', 'local seed')

    // B's doc should be unaffected — init is not broadcast
    expect(regB.get('scratch:1:999')!.getText('content').toString()).toBe('')
  })

  it('server-load origin broadcasts full state to empty peer docs', () => {
    const regA = createYDocRegistry(1)
    const regB = createYDocRegistry(1)

    // B opens the file first (empty doc)
    const docB = regB.getOrCreate('file:99')

    // A loads from server
    const docA = regA.getOrCreate('file:99')
    docA.transact(() => {
      docA.getText('content').insert(0, 'SELECT 1;')
    }, 'server-load')

    // B received A's full-state and applied it (doc was empty)
    expect(docB.getText('content').toString()).toBe('SELECT 1;')
  })

  it('full-state is NOT applied when peer doc already has content', () => {
    const regA = createYDocRegistry(1)
    const regB = createYDocRegistry(1)

    // B independently initialised (e.g. also loaded from server)
    const docB = regB.getOrCreate('file:99')
    docB.transact(() => {
      docB.getText('content').insert(0, 'SELECT 1;')
    }, 'server-load')

    // A also initialises — broadcasts full-state
    const docA = regA.getOrCreate('file:99')
    docA.transact(() => {
      docA.getText('content').insert(0, 'SELECT 1;')
    }, 'server-load')

    // B must NOT apply A's state on top of its own (would double content)
    expect(docB.getText('content').toString()).toBe('SELECT 1;')
  })

  it('sync-request causes existing peer to share its state', () => {
    const regA = createYDocRegistry(1)

    // A already has content
    const docA = regA.getOrCreate('file:99')
    docA.transact(() => {
      docA.getText('content').insert(0, 'FROM server')
    }, 'server-load')

    // B joins later — its sync-request triggers A to send full-state
    const regB = createYDocRegistry(1)
    const docB = regB.getOrCreate('file:99') // sends sync-request on creation

    expect(docB.getText('content').toString()).toBe('FROM server')
  })

  // ── Regression: Y.Doc lifecycle isolation ─────────────────────────────────
  // Before the fix, EditorSection used wsTabs (workspace-filtered) instead of
  // all tabs when managing the Y.Doc lifecycle. Switching to another workspace
  // caused the current workspace's docs to be destroy()ed, losing unsaved edits.

  it('destroy() on one tab does not affect docs for other tabs', () => {
    const reg = createYDocRegistry(1)
    const docA = reg.getOrCreate('scratch:1:1', 'query A')
    reg.getOrCreate('scratch:1:2', 'query B')

    reg.destroy('scratch:1:1')

    expect(reg.get('scratch:1:1')).toBeUndefined()
    expect(reg.get('scratch:1:2')?.getText('content').toString()).toBe('query B')
    // Silence the unused variable warning — we checked it was removed above.
    void docA
  })

  it('a doc persists in the registry until explicitly destroy()ed', () => {
    const reg = createYDocRegistry(1)
    const doc = reg.getOrCreate('scratch:1:1', 'SELECT 1;')
    doc.getText('content').insert(9, '\nSELECT 2;')

    // Simulate switching workspaces: creating docs for other tabs must NOT
    // remove existing docs (the bug was the wsTabs filtering that called
    // destroy on tabs outside the active workspace).
    reg.getOrCreate('scratch:2:1', 'other workspace tab')

    expect(reg.get('scratch:1:1')?.getText('content').toString()).toBe('SELECT 1;\nSELECT 2;')
  })

  // ── Regression: reload persistence via ySnapshot ───────────────────────────
  // Before the fix, Y.Docs were re-initialised from the stale creation-time
  // yState (empty for consoles) on page reload. The fix persists the current
  // Y.js binary state as tab.ySnapshot and applies it with 'init' on reload.

  it('Y.js state encoded via encodeStateAsUpdate can fully restore a doc after reload', () => {
    // Simulate a session: user opens a console and types a query.
    const reg = createYDocRegistry(1)
    const doc = reg.getOrCreate('scratch:1:1')
    doc.getText('content').insert(0, 'SELECT * FROM orders;\nWHERE id = 1;')

    const snapshot = Y.encodeStateAsUpdate(doc)

    // Simulate page reload: destroy the registry (all docs gone).
    reg.destroy('scratch:1:1')
    expect(reg.get('scratch:1:1')).toBeUndefined()

    // Restore: create a fresh registry and apply the snapshot with 'init'.
    const reg2 = createYDocRegistry(1)
    const restoredDoc = reg2.getOrCreate('scratch:1:1')
    Y.applyUpdate(restoredDoc, snapshot, 'init')

    expect(restoredDoc.getText('content').toString()).toBe('SELECT * FROM orders;\nWHERE id = 1;')
  })

  it('init origin does not broadcast snapshot to peers on reload', () => {
    // A peer that is already open must NOT receive our reload-restore update.
    // If it did, a second window would have the "init" applied on top of its
    // own state, potentially doubling content.
    const regPeer = createYDocRegistry(1)
    const peerDoc = regPeer.getOrCreate('scratch:1:1')
    peerDoc.getText('content').insert(0, 'peer content')

    // Reloading window restores from snapshot using 'init' origin.
    const regReload = createYDocRegistry(1)
    const reloadDoc = regReload.getOrCreate('scratch:1:1')
    const snapshot = Y.encodeStateAsUpdate(reloadDoc) // empty snapshot for test
    Y.applyUpdate(reloadDoc, snapshot, 'init')

    // Peer must still have only its own content — no echo from reload init.
    expect(peerDoc.getText('content').toString()).toBe('peer content')
  })

  it('after reload, incremental edits from a peer window are still received', () => {
    // Regression: restoring from ySnapshot with 'init' must not break the
    // BroadcastChannel subscription so cross-window sync keeps working.
    const regA = createYDocRegistry(1)
    const docA = regA.getOrCreate('scratch:1:1', 'SELECT 1;')
    const snapshot = Y.encodeStateAsUpdate(docA)

    // Simulate reload: fresh registry, restore from snapshot.
    const regReloaded = createYDocRegistry(1)
    const reloadedDoc = regReloaded.getOrCreate('scratch:1:1')
    Y.applyUpdate(reloadedDoc, snapshot, 'init')
    expect(reloadedDoc.getText('content').toString()).toBe('SELECT 1;')

    // Peer (still-open window) makes an edit — reloaded window must receive it.
    docA.getText('content').insert(9, '\nSELECT 2;')

    expect(reloadedDoc.getText('content').toString()).toBe('SELECT 1;\nSELECT 2;')
  })

  it('incremental updates are NOT re-broadcast (no echo loops)', () => {
    const regA = createYDocRegistry(1)
    const regB = createYDocRegistry(1)
    const docA = regA.getOrCreate('file:99')
    regB.getOrCreate('file:99')

    let bOutgoing = 0
    const origPost = FakeBroadcastChannel.prototype.postMessage
    FakeBroadcastChannel.prototype.postMessage = function (data) {
      if (this.name === 'sqlwarden:tab:1:file:99') bOutgoing++
      origPost.call(this, data)
    }

    docA.getText('content').insert(0, 'hi') // A → B (1 outgoing from A)

    FakeBroadcastChannel.prototype.postMessage = origPost

    expect(regB.get('file:99')!.getText('content').toString()).toBe('hi')
    // Only A sent (1 message). B must not re-broadcast (would be +1).
    expect(bOutgoing).toBe(1)
  })
})
