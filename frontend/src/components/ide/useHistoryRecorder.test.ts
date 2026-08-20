import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { createElement, type ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { queryKeys } from '#/lib/api/query-keys'
import * as queryHistoryApi from '#/lib/api/queries/query-history'
import * as localStore from './localQueryStore'
import { useHistoryRecorder } from './useHistoryRecorder'

function wrapper(client: QueryClient) {
  return ({ children }: { children: ReactNode }) =>
    createElement(QueryClientProvider, { client }, children)
}

describe('useHistoryRecorder', () => {
  it('records to backend when effective mode is backend', async () => {
    const client = new QueryClient()
    client.setQueryData(queryKeys.orgRuntimeSettings('acme'), {
      effective: { query_history_mode: 'backend' },
    })
    const recordSpy = vi
      .spyOn(queryHistoryApi, 'recordQueryHistoryEntry')
      .mockResolvedValue(undefined as never)
    const invalidateSpy = vi.spyOn(client, 'invalidateQueries')

    const { result } = renderHook(() => useHistoryRecorder('acme', 7, 42), {
      wrapper: wrapper(client),
    })
    result.current({ sqlText: 'select 1', status: 'ok', durationMs: 5, rowsAffected: 1 })

    expect(recordSpy).toHaveBeenCalledWith('acme', 7, 42, {
      sql_text: 'select 1',
      status: 'ok',
      error_message: undefined,
      duration_ms: 5,
      rows_affected: 1,
    })
    await waitFor(() =>
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: queryKeys.orgWorkspaceQueryHistoryScope('acme', 7),
      }),
    )
  })

  it('records locally when effective mode is local', async () => {
    const client = new QueryClient()
    client.setQueryData(queryKeys.orgRuntimeSettings('acme'), {
      effective: { query_history_mode: 'local', query_history_retention_count: 200 },
    })
    const localSpy = vi
      .spyOn(localStore, 'addLocalHistoryEntry')
      .mockResolvedValue(undefined as never)
    const invalidateSpy = vi.spyOn(client, 'invalidateQueries')

    const { result } = renderHook(() => useHistoryRecorder('acme', 7, 42), {
      wrapper: wrapper(client),
    })
    result.current({ sqlText: 'select 1', status: 'ok', durationMs: 5, rowsAffected: 1 })

    expect(localSpy).toHaveBeenCalledWith(
      {
        connectionId: 42,
        sqlText: 'select 1',
        status: 'ok',
        errorMessage: undefined,
        durationMs: 5,
        rowsAffected: 1,
      },
      200,
    )
    await waitFor(() =>
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: queryKeys.localQueryHistoryScope(7),
      }),
    )
  })

  it('records nothing when effective mode is off', () => {
    const client = new QueryClient()
    client.setQueryData(queryKeys.orgRuntimeSettings('acme'), {
      effective: { query_history_mode: 'off' },
    })
    const recordSpy = vi
      .spyOn(queryHistoryApi, 'recordQueryHistoryEntry')
      .mockResolvedValue(undefined as never)
    const localSpy = vi
      .spyOn(localStore, 'addLocalHistoryEntry')
      .mockResolvedValue(undefined as never)

    const { result } = renderHook(() => useHistoryRecorder('acme', 7, 42), {
      wrapper: wrapper(client),
    })
    result.current({ sqlText: 'select 1', status: 'ok', durationMs: 5, rowsAffected: 1 })

    expect(recordSpy).not.toHaveBeenCalled()
    expect(localSpy).not.toHaveBeenCalled()
  })
})
