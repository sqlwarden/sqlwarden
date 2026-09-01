import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ObjectRef } from '#/lib/api/types'
import {
  connectionDirectoryQueryKey,
  connectionObjectQueryKey,
  connectionRelationshipsQueryKey,
} from '#/lib/api/query'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import * as completion from './completion'
import { useSchemaRefresh } from './useSchemaRefresh'

const toastSuccess = vi.fn()
const toastError = vi.fn()
vi.mock('sonner', () => ({
  toast: {
    success: (...args: unknown[]) => toastSuccess(...args),
    error: (...args: unknown[]) => toastError(...args),
  },
}))

const ref: ObjectRef = {
  scope: [{ kind: 'schema', name: 'public' }],
  kind: 'table',
  name: 'users',
}

describe('useSchemaRefresh', () => {
  const queryClient = createTestQueryClient()

  beforeEach(() => {
    queryClient.clear()
    toastSuccess.mockClear()
    toastError.mockClear()
  })

  function wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }

  it('waits for a persistent refresh and invalidates the complete schema cache', async () => {
    let release!: () => void
    const pending = new Promise<void>((resolve) => {
      release = resolve
    })
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/refresh', async () => {
        await pending
        return HttpResponse.json({
          status: 'ok',
          mode: 'persistent',
          snapshot_id: 'snapshot-2',
          generated_at: '2026-08-06T00:00:00Z',
        })
      }),
    )
    const directoryKey = connectionDirectoryQueryKey('acme', 3, 7)
    const objectKey = connectionObjectQueryKey('acme', 3, 7, ref)
    const relationshipsKey = connectionRelationshipsQueryKey('acme', 3, 7, ref.scope)
    queryClient.setQueryData(directoryKey, { directory: {} })
    queryClient.setQueryData(objectKey, { ref })
    queryClient.setQueryData(relationshipsKey, { relationships: [] })

    const { result } = renderHook(
      () =>
        useSchemaRefresh({
          orgSlug: 'acme',
          workspaceId: 3,
          connectionId: 7,
          sessionId: 'session-7',
        }),
      { wrapper },
    )
    act(() => result.current.mutate())
    await waitFor(() => expect(result.current.isPending).toBe(true))
    release()
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(queryClient.getQueryState(directoryKey)?.isInvalidated).toBe(true)
    expect(queryClient.getQueryState(objectKey)?.isInvalidated).toBe(true)
    expect(queryClient.getQueryState(relationshipsKey)?.isInvalidated).toBe(true)
    expect(toastSuccess).toHaveBeenCalledWith('Schema refreshed')
  })

  it('keeps an ephemeral object refresh scoped to that object', async () => {
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/refresh', () =>
        HttpResponse.json({ status: 'ok', mode: 'ephemeral' }),
      ),
    )
    const directoryKey = connectionDirectoryQueryKey('acme', 3, 7)
    const objectKey = connectionObjectQueryKey('acme', 3, 7, ref)
    queryClient.setQueryData(directoryKey, { directory: {} })
    queryClient.setQueryData(objectKey, { ref })

    const { result } = renderHook(
      () =>
        useSchemaRefresh({
          orgSlug: 'acme',
          workspaceId: 3,
          connectionId: 7,
          sessionId: 'session-7',
          ref,
        }),
      { wrapper },
    )
    act(() => result.current.mutate())
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(queryClient.getQueryState(objectKey)?.isInvalidated).toBe(true)
    expect(queryClient.getQueryState(directoryKey)?.isInvalidated).toBe(false)
  })

  it('reports refresh failures without invalidating cached schema data', async () => {
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/refresh', () =>
        HttpResponse.json(
          { error: { code: 'schema_sync_timeout', message: 'Schema refresh timed out.' } },
          { status: 504 },
        ),
      ),
    )
    const directoryKey = connectionDirectoryQueryKey('acme', 3, 7)
    queryClient.setQueryData(directoryKey, { directory: {} })
    const { result } = renderHook(
      () => useSchemaRefresh({ orgSlug: 'acme', workspaceId: 3, connectionId: 7 }),
      { wrapper },
    )

    act(() => result.current.mutate())
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(queryClient.getQueryState(directoryKey)?.isInvalidated).toBe(false)
    expect(toastError).toHaveBeenCalledWith('Schema refresh timed out.')
  })

  it('invalidates the completion index after a successful schema refresh', async () => {
    const spy = vi.spyOn(completion, 'invalidateCompletionIndex')
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/refresh', () =>
        HttpResponse.json({
          status: 'ok',
          mode: 'persistent',
          snapshot_id: 'snapshot-3',
          generated_at: '2026-08-06T00:00:00Z',
        }),
      ),
    )

    const { result } = renderHook(
      () =>
        useSchemaRefresh({
          orgSlug: 'acme',
          workspaceId: 3,
          connectionId: 7,
          sessionId: 'session-7',
        }),
      { wrapper },
    )
    act(() => result.current.mutate())
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(spy).toHaveBeenCalledWith(7)
    spy.mockRestore()
  })
})
