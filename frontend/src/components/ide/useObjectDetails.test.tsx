import type { PropsWithChildren } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it } from 'vitest'
import type { ObjectRef } from '#/lib/api/types'
import { createTestQueryClient } from '#/test/render'
import { server } from '#/test/server'
import { useObjectDetails } from './useObjectDetails'

function tableRef(name: string): ObjectRef {
  return { scope: [{ kind: 'schema', name: 'public' }], kind: 'table', name }
}

describe('useObjectDetails', () => {
  const queryClient = createTestQueryClient()

  beforeEach(() => {
    queryClient.clear()
  })

  function wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  }

  it('groups refs into chunks instead of one request per ref', async () => {
    const requests: ObjectRef[][] = []
    server.use(
      http.post('/api/v1/orgs/acme/workspaces/3/connections/7/schema/objects', async ({ request }) => {
        const body = (await request.json()) as { refs: ObjectRef[] }
        requests.push(body.refs)
        return HttpResponse.json({
          objects: body.refs.map((ref) => ({ ref, relational: { columns: [] } })),
        })
      }),
    )

    const refs = Array.from({ length: 5 }, (_, i) => tableRef(`t${i}`))
    const { result } = renderHook(
      () =>
        useObjectDetails({
          orgSlug: 'acme',
          workspaceId: 3,
          connectionId: 7,
          sessionId: 'sess-1',
          refs,
          enabled: true,
          chunkSize: 2,
          concurrency: 5,
        }),
      { wrapper },
    )

    await waitFor(() => {
      expect(result.current.byRef.get('[{"kind":"schema","name":"public"}]:table:t4')?.detail).not.toBeNull()
    })

    // 5 refs at chunk size 2 -> 3 requests, not 5.
    expect(requests).toHaveLength(3)
    expect(requests.map((r) => r.length)).toEqual([2, 2, 1])
  })

  it('limits how many chunks are in flight at once', async () => {
    let inFlight = 0
    let maxInFlight = 0
    const releases: (() => void)[] = []
    server.use(
      http.post(
        '/api/v1/orgs/acme/workspaces/3/connections/7/schema/objects',
        async ({ request }) => {
          inFlight += 1
          maxInFlight = Math.max(maxInFlight, inFlight)
          const body = (await request.json()) as { refs: ObjectRef[] }
          await new Promise<void>((resolve) => releases.push(resolve))
          inFlight -= 1
          return HttpResponse.json({
            objects: body.refs.map((ref) => ({ ref, relational: { columns: [] } })),
          })
        },
      ),
    )

    const refs = Array.from({ length: 6 }, (_, i) => tableRef(`t${i}`))
    renderHook(
      () =>
        useObjectDetails({
          orgSlug: 'acme',
          workspaceId: 3,
          connectionId: 7,
          sessionId: 'sess-1',
          refs,
          enabled: true,
          chunkSize: 1,
          concurrency: 2,
        }),
      { wrapper },
    )

    await waitFor(() => expect(releases.length).toBe(2))
    expect(maxInFlight).toBe(2)
    releases.splice(0).forEach((release) => release())

    await waitFor(() => expect(releases.length).toBeGreaterThan(0))
    expect(maxInFlight).toBe(2)
    releases.splice(0).forEach((release) => release())
  })
})
