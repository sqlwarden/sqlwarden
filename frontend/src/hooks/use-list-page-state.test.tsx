// @vitest-environment jsdom

import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useListPageState } from './use-list-page-state'

afterEach(() => {
  vi.useRealTimers()
})

describe('useListPageState', () => {
  it('resets pagination when debounced search changes', () => {
    vi.useFakeTimers()
    const { result } = renderHook(() =>
      useListPageState({ page: 4, page_size: 25, sort: 'name', order: 'asc' }),
    )

    act(() => result.current.setSearchText('  active  '))
    act(() => vi.advanceTimersByTime(300))

    expect(result.current.query).toMatchObject({ page: 1, page_size: 25, q: 'active' })
  })

  it('resets the page when page size changes', () => {
    const { result } = renderHook(() => useListPageState({ page: 3, page_size: 10 }))
    act(() => result.current.setPageSize(50))
    expect(result.current.query).toMatchObject({ page: 1, page_size: 50 })
  })

  it('toggles the active sort and starts a new sort ascending', () => {
    const { result } = renderHook(() => useListPageState({ page: 2, sort: 'name', order: 'asc' }))

    act(() => result.current.toggleSort('name'))
    expect(result.current.query).toMatchObject({ page: 1, sort: 'name', order: 'desc' })

    act(() => result.current.toggleSort('created_at'))
    expect(result.current.query).toMatchObject({ page: 1, sort: 'created_at', order: 'asc' })
  })
})
