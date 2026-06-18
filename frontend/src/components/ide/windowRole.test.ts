import { describe, it, expect, vi } from 'vitest'
import { makeRoleGatedStorage } from './windowRole'

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
  })
})
