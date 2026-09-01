import { afterEach, expect, it, vi } from 'vitest'
import { getConnectionCompletionIndex } from './database'

afterEach(() => vi.unstubAllGlobals())

it('requests the completion-index endpoint with the session header and unwraps the body', async () => {
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    expect(String(input)).toContain(
      '/api/v1/orgs/acme/workspaces/3/connections/7/schema/completion-index',
    )
    expect(new Headers(init?.headers).get('X-Warden-Session')).toBe('sess-1')
    return new Response(
      JSON.stringify({
        version: 'snap-1',
        default_schema: 'public',
        schemas: ['public'],
        objects: [{ schema: 'public', name: 'orders', kind: 'table' }],
        columns: [{ schema: 'public', table: 'orders', name: 'id', type: 'int8', nullable: false }],
      }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    )
  })
  vi.stubGlobal('fetch', fetchMock)

  const res = await getConnectionCompletionIndex('acme', 3, 7, 'sess-1')

  expect(res.default_schema).toBe('public')
  expect(res.objects[0]).toEqual({ schema: 'public', name: 'orders', kind: 'table' })
  expect(res.columns[0].nullable).toBe(false)
})
