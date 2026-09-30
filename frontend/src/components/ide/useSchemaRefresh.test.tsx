import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { queryKeys } from '#/lib/api/query-keys'
import type { NavigatorListing, ObjectRef } from '#/lib/api/types'
import { connectionObjectQueryKey } from '#/lib/api/query'
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

const schemaPath = [
  { kind: 'database', name: 'app' },
  { kind: 'schema', name: 'public' },
]
const ref: ObjectRef = { scope: schemaPath, kind: 'table', name: 'users' }
const refreshURL = '/api/v1/orgs/acme/workspaces/3/connections/7/schema/refresh'

function tables(names: string[]): NavigatorListing {
  return {
    path: schemaPath,
    folder: 'tables',
    items: names.map((name) => ({
      kind: 'table',
      name,
      path: [...schemaPath, { kind: 'table', name }],
      system: false,
      current: false,
    })),
    fetched_at: '2026-09-29T00:00:00Z',
    source: 'live',
  }
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

  it('posts the node path with the session and writes the returned listings', async () => {
    let body: unknown
    let session: string | null = null
    server.use(
      http.post(refreshURL, async ({ request }) => {
        body = await request.json()
        session = request.headers.get('X-Warden-Session')
        return HttpResponse.json({ items: [tables(['orders', 'users'])] })
      }),
    )
    const tablesKey = queryKeys.connectionSchemaNodes('acme', 3, 7, schemaPath, 'tables')
    queryClient.setQueryData(tablesKey, tables(['users']))
    const completionSpy = vi.spyOn(completion, 'invalidateCompletionIndex')

    const { result } = renderHook(
      () =>
        useSchemaRefresh({
          orgSlug: 'acme',
          workspaceId: 3,
          connectionId: 7,
          sessionId: 'session-7',
          path: schemaPath,
        }),
      { wrapper },
    )
    act(() => result.current.mutate())
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(body).toEqual({ path: schemaPath })
    expect(session).toBe('session-7')
    expect(
      queryClient.getQueryData<NavigatorListing>(tablesKey)?.items.map((item) => item.name),
    ).toEqual(['orders', 'users'])
    expect(completionSpy).toHaveBeenCalledWith(7)
    expect(toastSuccess).toHaveBeenCalledWith('Schema refreshed')
  })

  it('refreshes the connection root when no path is given and invalidates object details', async () => {
    let body: unknown
    server.use(
      http.post(refreshURL, async ({ request }) => {
        body = await request.json()
        return HttpResponse.json({ items: [] })
      }),
    )
    const objectKey = connectionObjectQueryKey('acme', 3, 7, ref, 'session-7')
    queryClient.setQueryData(objectKey, { detail: null })

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

    expect(body).toEqual({ path: [] })
    expect(queryClient.getQueryState(objectKey)?.isInvalidated).toBe(true)
  })

  it('refreshes and prunes a per-call target instead of the hook path', async () => {
    let body: unknown
    server.use(
      http.post(refreshURL, async ({ request }) => {
        body = await request.json()
        return HttpResponse.json({ items: [] })
      }),
    )
    const usersPath = [...schemaPath, { kind: 'table', name: 'users' }]
    const columnsKey = queryKeys.connectionSchemaNodes('acme', 3, 7, usersPath, 'columns')
    const tablesKey = queryKeys.connectionSchemaNodes('acme', 3, 7, schemaPath, 'tables')
    queryClient.setQueryData(columnsKey, { ...tables([]), path: usersPath, folder: 'columns' })
    queryClient.setQueryData(tablesKey, tables(['users']))

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
    act(() => result.current.mutate(usersPath))
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(body).toEqual({ path: usersPath })
    expect(queryClient.getQueryData(columnsKey)).toBeUndefined()
    expect(queryClient.getQueryData(tablesKey)).toBeDefined()
  })

  it('shows the session_required message without touching cached listings', async () => {
    server.use(
      http.post(refreshURL, () =>
        HttpResponse.json(
          {
            error: {
              code: 'session_required',
              message: 'Connect to this database to load schema objects.',
            },
          },
          { status: 409 },
        ),
      ),
    )
    const tablesKey = queryKeys.connectionSchemaNodes('acme', 3, 7, schemaPath, 'tables')
    queryClient.setQueryData(tablesKey, tables(['users']))

    const { result } = renderHook(
      () =>
        useSchemaRefresh({ orgSlug: 'acme', workspaceId: 3, connectionId: 7, path: schemaPath }),
      { wrapper },
    )
    act(() => result.current.mutate())
    await waitFor(() => expect(result.current.isError).toBe(true))

    expect(toastError).toHaveBeenCalledWith('Connect to this database to load schema objects.')
    expect(queryClient.getQueryData<NavigatorListing>(tablesKey)?.items).toHaveLength(1)
  })
})
