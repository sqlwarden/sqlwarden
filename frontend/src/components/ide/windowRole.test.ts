import { afterEach, describe, it, expect, vi } from 'vitest'
import { electPrimary, makeRoleGatedStorage } from './windowRole'

afterEach(() => vi.unstubAllGlobals())

describe('makeRoleGatedStorage', () => {
  it('always reads, but only writes when canPersist() is true', async () => {
    const backing = new Map<string, string>()
    const store = {
      get: vi.fn(async (k: string) => backing.get(k) ?? null),
      set: vi.fn(async (k: string, v: string) => {
        backing.set(k, v)
      }),
      del: vi.fn(async (k: string) => {
        backing.delete(k)
      }),
    }
    let primary = false
    const s = makeRoleGatedStorage('k', () => primary, store)

    backing.set('k', 'seed')
    expect(await s.getItem('k')).toBe('seed') // read allowed while secondary

    await s.setItem('k', 'A')
    expect(store.set).not.toHaveBeenCalled() // write blocked while secondary
    expect(backing.get('k')).toBe('seed')

    primary = true
    await s.setItem('k', 'B')
    expect(backing.get('k')).toBe('B') // write allowed once primary

    await s.removeItem('k')
    expect(backing.has('k')).toBe(false)
  })
})

describe('electPrimary', () => {
  it('becomes primary immediately when Web Locks are unavailable', () => {
    vi.stubGlobal('navigator', {})
    const onBecamePrimary = vi.fn()

    const cleanup = electPrimary('ide', onBecamePrimary)

    expect(onBecamePrimary).toHaveBeenCalledOnce()
    expect(cleanup).not.toThrow()
  })

  it('holds the acquired lock until cleanup releases it', async () => {
    let lockCallback: (() => Promise<void>) | undefined
    const request = vi.fn((_name: string, _options: LockOptions, callback: () => Promise<void>) => {
      lockCallback = callback
      return Promise.resolve()
    })
    vi.stubGlobal('navigator', { locks: { request } })
    const onBecamePrimary = vi.fn()

    const cleanup = electPrimary('ide', onBecamePrimary)
    expect(request).toHaveBeenCalledWith(
      'ide',
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
      expect.any(Function),
    )

    const held = lockCallback?.()
    expect(onBecamePrimary).toHaveBeenCalledOnce()
    let released = false
    void held?.then(() => {
      released = true
    })
    await Promise.resolve()
    expect(released).toBe(false)

    cleanup()
    await held
    expect(released).toBe(true)
  })
})
