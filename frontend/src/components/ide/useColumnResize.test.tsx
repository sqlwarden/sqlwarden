import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { useColumnResize } from './useColumnResize'

function mouseEvent(clientX: number) {
  return { clientX, preventDefault() {} } as React.MouseEvent
}

describe('useColumnResize', () => {
  it('resizes the selected column and enforces its minimum width', () => {
    const { result } = renderHook(() => useColumnResize(2, 150, 60))

    act(() => result.current.startResize(mouseEvent(100), 1))
    act(() => window.dispatchEvent(new MouseEvent('mousemove', { clientX: 180 })))
    expect(result.current.columnWidths).toEqual([150, 230])

    act(() => window.dispatchEvent(new MouseEvent('mousemove', { clientX: -100 })))
    expect(result.current.columnWidths).toEqual([150, 60])
  })

  it('accepts per-column default widths', () => {
    const { result } = renderHook(() => useColumnResize(2, [100, 200], 60))
    expect(result.current.columnWidths).toEqual([100, 200])
  })

  it('persists resized widths under a cache key across remounts of the same shape', () => {
    const key = `shape-${Math.random()}`
    const first = renderHook(() => useColumnResize(2, [100, 200], 60, key))
    act(() => first.result.current.startResize(mouseEvent(100), 1))
    act(() => window.dispatchEvent(new MouseEvent('mousemove', { clientX: 150 })))
    expect(first.result.current.columnWidths).toEqual([100, 250])
    first.unmount()

    const second = renderHook(() => useColumnResize(2, [100, 200], 60, key))
    expect(second.result.current.columnWidths).toEqual([100, 250])
  })

  it('falls back to fresh defaults when a cached shape has a different column count', () => {
    const key = `shape-${Math.random()}`
    const first = renderHook(() => useColumnResize(2, [100, 200], 60, key))
    act(() => first.result.current.startResize(mouseEvent(100), 1))
    act(() => window.dispatchEvent(new MouseEvent('mousemove', { clientX: 150 })))
    first.unmount()

    const second = renderHook(() => useColumnResize(3, [100, 200, 300], 60, key))
    expect(second.result.current.columnWidths).toEqual([100, 200, 300])
  })

  it('evicts the least-recently-used shape once the cache exceeds its cap', () => {
    const prefix = `evict-${Math.random()}`

    // Fill the cache past its 200-entry cap with distinct shapes.
    for (let i = 0; i < 200; i++) {
      const hook = renderHook(() => useColumnResize(1, 100, 60, `${prefix}-${i}`))
      act(() => hook.result.current.startResize(mouseEvent(0), 0))
      act(() => window.dispatchEvent(new MouseEvent('mousemove', { clientX: 10 })))
      hook.unmount()
    }

    // Touching one more shape should evict the oldest (`${prefix}-0`) rather
    // than growing the cache unboundedly.
    const overflow = renderHook(() => useColumnResize(1, 100, 60, `${prefix}-overflow`))
    act(() => overflow.result.current.startResize(mouseEvent(0), 0))
    act(() => window.dispatchEvent(new MouseEvent('mousemove', { clientX: 10 })))
    overflow.unmount()

    const evicted = renderHook(() => useColumnResize(1, 100, 60, `${prefix}-0`))
    expect(evicted.result.current.columnWidths).toEqual([100])

    const retained = renderHook(() => useColumnResize(1, 100, 60, `${prefix}-199`))
    expect(retained.result.current.columnWidths).toEqual([110])
  })

  it('restores document interaction styles after mouseup and unmount', () => {
    const { result, unmount } = renderHook(() => useColumnResize(1, 150, 60))

    act(() => result.current.startResize(mouseEvent(100), 0))
    expect(document.body.style.cursor).toBe('col-resize')
    act(() => window.dispatchEvent(new MouseEvent('mouseup')))
    expect(document.body.style.cursor).toBe('')

    act(() => result.current.startResize(mouseEvent(100), 0))
    unmount()
    expect(document.body.style.cursor).toBe('')
    expect(document.body.style.userSelect).toBe('')
  })
})
