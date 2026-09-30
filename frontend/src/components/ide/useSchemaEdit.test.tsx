import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { queryKeys } from '#/lib/api/query-keys'
import type { NavigatorListing } from '#/lib/api/types'
import { connectionObjectQueryKey, connectionRelationshipsQueryKey } from '#/lib/api/query'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { useSchemaEdit } from './useSchemaEdit'

const toastWarning = vi.fn()
const toastError = vi.fn()
vi.mock('sonner', () => ({
  toast: {
    warning: (...args: unknown[]) => toastWarning(...args),
    error: (...args: unknown[]) => toastError(...args),
  },
}))

describe('useSchemaEdit', () => {
  const queryClient = createTestQueryClient()

  beforeEach(() => {
    queryClient.clear()
    toastWarning.mockClear()
    toastError.mockClear()
  })

  function wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }

  it('applies the mutation with the session header and invalidates the schema cache', async () => {
    let receivedHeader: string | null = null
    server.use(
      http.post(
        '/api/v1/orgs/acme/workspaces/3/connections/7/schema/mutations',
        async ({ request }) => {
          receivedHeader = request.headers.get('X-Warden-Session')
          return HttpResponse.json({
            applied: true,
            schema: { status: 'available', mode: 'persistent' },
            transaction: { mode: 'auto', open: false, pending_statements: 0 },
          })
        },
      ),
    )
    const objectKey = connectionObjectQueryKey('acme', 3, 7, baseRef)
    const relationshipsKey = connectionRelationshipsQueryKey('acme', 3, 7, baseRef.scope)
    queryClient.setQueryData(objectKey, { ref: baseRef })
    queryClient.setQueryData(relationshipsKey, { relationships: [] })

    const { result } = renderHook(
      () =>
        useSchemaEdit({ orgSlug: 'acme', workspaceId: 3, connectionId: 7, sessionId: 'session-7' }),
      { wrapper },
    )
    act(() =>
      result.current.mutate({
        operation: 'create_table',
        scope: [{ kind: 'schema', name: 'public' }],
        name: 'orders',
        columns: [{ name: 'id', data_type: 'integer', nullable: false, primary_key: true }],
      }),
    )
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(receivedHeader).toBe('session-7')
    expect(queryClient.getQueryState(objectKey)?.isInvalidated).toBe(true)
    expect(queryClient.getQueryState(relationshipsKey)?.isInvalidated).toBe(true)
    expect(toastWarning).not.toHaveBeenCalled()
  })

  it('warns without failing when the DDL applied but the metadata refresh failed', async () => {
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/mutations', () =>
        HttpResponse.json({
          applied: true,
          schema: { status: 'refresh_failed', mode: 'persistent' },
          transaction: { mode: 'auto', open: false, pending_statements: 0 },
        }),
      ),
    )

    const { result } = renderHook(
      () =>
        useSchemaEdit({ orgSlug: 'acme', workspaceId: 3, connectionId: 7, sessionId: 'session-7' }),
      { wrapper },
    )
    act(() => result.current.mutate({ operation: 'drop_object', ref: baseRef }))
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(toastWarning).toHaveBeenCalledWith(
      'Change applied, but refreshing the schema failed. Use Refresh to see the update.',
    )
  })

  it('rejects without a request when there is no live session', async () => {
    const { result } = renderHook(
      () => useSchemaEdit({ orgSlug: 'acme', workspaceId: 3, connectionId: 7 }),
      { wrapper },
    )
    act(() => result.current.mutate({ operation: 'drop_object', ref: baseRef }))
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(toastError).toHaveBeenCalledWith('Connect to the database to make this change.')
  })

  it('surfaces backend errors through the standard error toast', async () => {
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/mutations', () =>
        HttpResponse.json(
          { error: { code: 'validation_failed', message: 'Table already exists.' } },
          { status: 422 },
        ),
      ),
    )

    const { result } = renderHook(
      () =>
        useSchemaEdit({ orgSlug: 'acme', workspaceId: 3, connectionId: 7, sessionId: 'session-7' }),
      { wrapper },
    )
    act(() => result.current.mutate({ operation: 'drop_object', ref: baseRef }))
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(toastError).toHaveBeenCalledWith('Table already exists.')
  })

  it('writes the refreshed listings returned with a successful mutation', async () => {
    const schemaPath = [
      { kind: 'database', name: 'app' },
      { kind: 'schema', name: 'public' },
    ]
    const refreshed: NavigatorListing = {
      path: schemaPath,
      folder: 'tables',
      items: [
        {
          kind: 'table',
          name: 'orders',
          path: [...schemaPath, { kind: 'table', name: 'orders' }],
          system: false,
          current: false,
        },
      ],
      fetched_at: '2026-09-29T00:00:00Z',
      source: 'live',
    }
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/mutations', () =>
        HttpResponse.json({
          applied: true,
          schema: { status: 'available', mode: 'persistent' },
          transaction: { mode: 'auto', open: false, pending_statements: 0 },
          listings: [refreshed],
        }),
      ),
    )
    const tablesKey = queryKeys.connectionSchemaNodes('acme', 3, 7, schemaPath, 'tables')
    queryClient.setQueryData(tablesKey, { ...refreshed, items: [] })

    const { result } = renderHook(
      () =>
        useSchemaEdit({ orgSlug: 'acme', workspaceId: 3, connectionId: 7, sessionId: 'session-7' }),
      { wrapper },
    )
    act(() =>
      result.current.mutate({
        operation: 'create_table',
        scope: schemaPath,
        name: 'orders',
        columns: [{ name: 'id', data_type: 'integer', nullable: false, primary_key: true }],
      }),
    )
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(
      queryClient.getQueryData<NavigatorListing>(tablesKey)?.items.map((item) => item.name),
    ).toEqual(['orders'])
  })
})

const baseRef = {
  scope: [{ kind: 'schema', name: 'public' }],
  kind: 'table',
  name: 'orders',
}
