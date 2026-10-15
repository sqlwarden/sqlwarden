import { act, renderHook } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { useSecretReveal } from './useSecretReveal'

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((r) => {
    resolve = r
  })
  return { promise, resolve }
}

describe('useSecretReveal', () => {
  it('holds the revealed value until hidden', async () => {
    const reveal = vi.fn().mockResolvedValue('hunter2')
    const { result } = renderHook(() => useSecretReveal(reveal))

    await act(() => result.current.show())
    expect(result.current.value).toBe('hunter2')

    act(() => result.current.hide())
    expect(result.current.value).toBeNull()
  })

  it('drops a value that resolves after hide', async () => {
    const pending = deferred<string>()
    const { result } = renderHook(() => useSecretReveal(() => pending.promise))

    let shown!: Promise<void>
    act(() => {
      shown = result.current.show()
    })
    expect(result.current.pending).toBe(true)
    act(() => result.current.hide())
    await act(async () => {
      pending.resolve('late')
      await shown
    })

    expect(result.current.value).toBeNull()
    expect(result.current.pending).toBe(false)
  })

  it('stays hidden when the request fails', async () => {
    const reveal = vi.fn().mockRejectedValue(new Error('denied'))
    const { result } = renderHook(() => useSecretReveal(reveal))

    await act(() => result.current.show())

    expect(result.current.value).toBeNull()
    expect(result.current.pending).toBe(false)
  })

  it('does nothing without a reveal function', async () => {
    const { result } = renderHook(() => useSecretReveal(undefined))
    await act(() => result.current.show())
    expect(result.current.value).toBeNull()
  })
})
