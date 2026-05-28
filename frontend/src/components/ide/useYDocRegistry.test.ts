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

// Install fake before any tests run.
;(globalThis as unknown as Record<string, unknown>).BroadcastChannel = FakeBroadcastChannel

beforeEach(() => channels.clear())

describe('createYDocRegistry', () => {
  it('getOrCreate returns a Y.Doc', () => {
    const reg = createYDocRegistry()
    expect(reg.getOrCreate('tab:1')).toBeInstanceOf(Y.Doc)
  })

  it('getOrCreate is idempotent — returns the same doc', () => {
    const reg = createYDocRegistry()
    expect(reg.getOrCreate('tab:1')).toBe(reg.getOrCreate('tab:1'))
  })

  it('getOrCreate initialises content from initialContent', () => {
    const reg = createYDocRegistry()
    const doc = reg.getOrCreate('tab:1', 'SELECT 1;')
    expect(doc.getText('content').toString()).toBe('SELECT 1;')
  })

  it('get returns undefined before creation', () => {
    expect(createYDocRegistry().get('nope')).toBeUndefined()
  })

  it('get returns the doc after creation', () => {
    const reg = createYDocRegistry()
    reg.getOrCreate('tab:1')
    expect(reg.get('tab:1')).toBeInstanceOf(Y.Doc)
  })

  it('destroy removes the doc', () => {
    const reg = createYDocRegistry()
    reg.getOrCreate('tab:1')
    reg.destroy('tab:1')
    expect(reg.get('tab:1')).toBeUndefined()
  })

  it('destroy is a no-op for unknown tabId', () => {
    expect(() => createYDocRegistry().destroy('nope')).not.toThrow()
  })

  it('user edits are broadcast to another window on the same tabId', () => {
    const regA = createYDocRegistry()
    const regB = createYDocRegistry()
    const docA = regA.getOrCreate('file:99')
    const docB = regB.getOrCreate('file:99')

    docA.getText('content').insert(0, 'hello')

    expect(docB.getText('content').toString()).toBe('hello')
  })

  it('server-load origin is NOT broadcast', () => {
    const regA = createYDocRegistry()
    const regB = createYDocRegistry()
    regB.getOrCreate('file:99')
    const docA = regA.getOrCreate('file:99')

    docA.transact(() => {
      docA.getText('content').insert(0, 'server content')
    }, 'server-load')

    expect(regB.get('file:99')!.getText('content').toString()).toBe('')
  })

  it('init origin is NOT broadcast', () => {
    // docB exists before docA is created with initialContent
    const regB = createYDocRegistry()
    regB.getOrCreate('file:99')

    const regA = createYDocRegistry()
    regA.getOrCreate('file:99', 'initial') // uses 'init' origin internally

    // docB should not have received the init insert
    expect(regB.get('file:99')!.getText('content').toString()).toBe('')
  })

  it('broadcast updates are NOT re-broadcast (no echo loops)', () => {
    const regA = createYDocRegistry()
    const regB = createYDocRegistry()
    const docA = regA.getOrCreate('file:99')
    regB.getOrCreate('file:99')

    // Count outgoing posts from regB's channel after A sends
    let bOutgoing = 0
    const origPost = FakeBroadcastChannel.prototype.postMessage
    FakeBroadcastChannel.prototype.postMessage = function (data) {
      // Only count posts from channels that belong to regB
      if (this.name === 'sqlwarden:tab:file:99') bOutgoing++
      origPost.call(this, data)
    }

    docA.getText('content').insert(0, 'hi') // A → B

    FakeBroadcastChannel.prototype.postMessage = origPost

    // B received the update correctly
    expect(regB.get('file:99')!.getText('content').toString()).toBe('hi')
    // B must not have re-broadcast (bOutgoing counts A's send too, but B's re-send would be +1)
    // A sent 1 message; B should have sent 0. Total = 1 (from A).
    expect(bOutgoing).toBe(1)
  })
})
