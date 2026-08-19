// @vitest-environment jsdom

import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { WorkspaceFileSearchResult } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { useFileContentSearch } from './useFileContentSearch'

afterEach(() => {
  vi.useRealTimers()
})

function emptyResult(query: string, filesScanned: number): WorkspaceFileSearchResult {
  return { query, results: [], files_scanned: filesScanned, truncated: false }
}

describe('useFileContentSearch', () => {
  function wrapper({ children }: PropsWithChildren) {
    const queryClient = createTestQueryClient()
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }

  it('gates both searches behind the minimum query length and debounce delay', () => {
    vi.useFakeTimers()
    const { result } = renderHook(() => useFileContentSearch('acme', 3), { wrapper })

    expect(result.current.private.fetchStatus).toBe('idle')
    expect(result.current.shared.fetchStatus).toBe('idle')

    act(() => result.current.setSearchText('o'))
    act(() => vi.advanceTimersByTime(300))
    expect(result.current.isQueryTooShort).toBe(true)
    expect(result.current.private.fetchStatus).toBe('idle')

    act(() => result.current.setSearchText('or'))
    expect(result.current.private.fetchStatus).toBe('idle')
    act(() => vi.advanceTimersByTime(299))
    expect(result.current.private.fetchStatus).toBe('idle')
  })

  it('fires parallel private and shared searches once the query is ready', async () => {
    server.use(
      http.get('/api/v1/orgs/acme/workspaces/3/files/private/search', ({ request }) => {
        const q = new URL(request.url).searchParams.get('q')
        return HttpResponse.json(emptyResult(q ?? '', 2))
      }),
      http.get('/api/v1/orgs/acme/workspaces/3/files/shared/search', ({ request }) => {
        const q = new URL(request.url).searchParams.get('q')
        return HttpResponse.json(emptyResult(q ?? '', 5))
      }),
    )
    vi.useFakeTimers()
    const { result } = renderHook(() => useFileContentSearch('acme', 3), { wrapper })

    act(() => result.current.setSearchText('orders'))
    act(() => vi.advanceTimersByTime(300))
    vi.useRealTimers()

    await waitFor(() => expect(result.current.private.data?.files_scanned).toBe(2))
    await waitFor(() => expect(result.current.shared.data?.files_scanned).toBe(5))
    expect(result.current.debouncedQuery).toBe('orders')
    expect(result.current.isQueryTooShort).toBe(false)
  })

  it('clearSearch resets the query text and debounced query', () => {
    vi.useFakeTimers()
    const { result } = renderHook(() => useFileContentSearch('acme', 3), { wrapper })

    act(() => result.current.setSearchText('orders'))
    act(() => vi.advanceTimersByTime(300))
    expect(result.current.debouncedQuery).toBe('orders')

    act(() => result.current.clearSearch())
    expect(result.current.searchText).toBe('')
    act(() => vi.advanceTimersByTime(300))
    expect(result.current.debouncedQuery).toBe('')
    expect(result.current.private.fetchStatus).toBe('idle')
  })
})
