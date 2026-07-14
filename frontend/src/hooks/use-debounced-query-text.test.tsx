// @vitest-environment jsdom

import { act, renderHook } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useDebouncedQueryText } from './use-debounced-query-text'

afterEach(() => {
  vi.useRealTimers()
})

describe('useDebouncedQueryText', () => {
  it('trims and publishes text after the configured delay', () => {
    vi.useFakeTimers()
    const { result } = renderHook(() => useDebouncedQueryText('', 250))

    act(() => result.current.setSearchText('  invoices  '))
    expect(result.current.debouncedQuery).toBe('')

    act(() => vi.advanceTimersByTime(249))
    expect(result.current.debouncedQuery).toBe('')

    act(() => vi.advanceTimersByTime(1))
    expect(result.current.debouncedQuery).toBe('invoices')
  })

  it('cancels the previous timer and supports clearing the input', () => {
    vi.useFakeTimers()
    const { result } = renderHook(() => useDebouncedQueryText('initial', 100))

    act(() => result.current.setSearchText('first'))
    act(() => vi.advanceTimersByTime(50))
    act(() => result.current.setSearchText('second'))
    act(() => vi.advanceTimersByTime(100))
    expect(result.current.debouncedQuery).toBe('second')

    act(() => result.current.clearSearch())
    expect(result.current.searchText).toBe('')
    act(() => vi.advanceTimersByTime(100))
    expect(result.current.debouncedQuery).toBe('')
  })
})
