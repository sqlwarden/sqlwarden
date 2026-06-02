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

  it('incremental updates are NOT re-broadcast (no echo loops)', () => {
    const regA = createYDocRegistry(1)
    const regB = createYDocRegistry(1)
    const docA = regA.getOrCreate('file:99')
    regB.getOrCreate('file:99')

    let bOutgoing = 0
    const origPost = FakeBroadcastChannel.prototype.postMessage
    FakeBroadcastChannel.prototype.postMessage = function (data) {
      if (this.name === 'sqlwarden:tab:file:99') bOutgoing++
      origPost.call(this, data)
    }

    docA.getText('content').insert(0, 'hi') // A → B (1 outgoing from A)

    FakeBroadcastChannel.prototype.postMessage = origPost

    expect(regB.get('file:99')!.getText('content').toString()).toBe('hi')
    // Only A sent (1 message). B must not re-broadcast (would be +1).
    expect(bOutgoing).toBe(1)
  })
})
