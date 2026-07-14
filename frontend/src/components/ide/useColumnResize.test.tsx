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
