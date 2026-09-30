import { afterEach, expect, it, vi } from 'vitest'
import { QueryClient } from '@tanstack/react-query'
import { isSessionRequired } from '#/lib/api/errors'
import { queryKeys } from '#/lib/api/query-keys'
import type { NavigatorListing } from '#/lib/api/types'
import {
  applyNavigatorListings,
  getConnectionCompletionIndex,
  schemaNodesQueryOptions,
  scopePathWithin,
} from './database'

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

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

const schemaPath = [
  { kind: 'database', name: 'app' },
  { kind: 'schema', name: 'public' },
]

function listing(
  path: NavigatorListing['path'],
  folder: string,
  names: string[],
): NavigatorListing {
  return {
    path,
    folder,
    items: names.map((name) => ({
      kind: 'table',
      name,
      path: [...path, { kind: 'table', name }],
      system: false,
      current: false,
    })),
    fetched_at: '2026-09-29T00:00:00Z',
    source: 'live',
  }
}

it('requests one folder listing with the encoded path, folder, and session header', async () => {
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(String(input), 'http://localhost')
    expect(url.pathname).toBe('/api/v1/orgs/acme/workspaces/3/connections/7/schema/nodes')
    expect(url.searchParams.get('folder')).toBe('tables')
    expect(JSON.parse(url.searchParams.get('path') ?? '')).toEqual(schemaPath)
    expect(new Headers(init?.headers).get('X-Warden-Session')).toBe('sess-1')
    return jsonResponse(listing(schemaPath, 'tables', ['orders']))
  })
  vi.stubGlobal('fetch', fetchMock)

  const client = new QueryClient()
  const res = await client.fetchQuery(
    schemaNodesQueryOptions('acme', 3, 7, schemaPath, 'tables', 'sess-1'),
  )

  expect(res.items.map((item) => item.name)).toEqual(['orders'])
})

it('omits the path parameter for root folders', async () => {
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    expect(new URL(String(input), 'http://localhost').searchParams.has('path')).toBe(false)
    return jsonResponse(listing([], 'databases', []))
  })
  vi.stubGlobal('fetch', fetchMock)

  await new QueryClient().fetchQuery(schemaNodesQueryOptions('acme', 3, 7, [], 'databases'))
  expect(fetchMock).toHaveBeenCalledTimes(1)
})

it('surfaces session_required as a non-retried error', async () => {
  const fetchMock = vi.fn(async () =>
    jsonResponse(
      {
        error: {
          code: 'session_required',
          message: 'Connect to this database to load schema objects.',
        },
      },
      409,
    ),
  )
  vi.stubGlobal('fetch', fetchMock)

  const client = new QueryClient()
  const error = await client
    .fetchQuery(schemaNodesQueryOptions('acme', 3, 7, schemaPath, 'tables'))
    .catch((err: unknown) => err)

  expect(isSessionRequired(error)).toBe(true)
  expect(fetchMock).toHaveBeenCalledTimes(1)
})

it('matches scope paths by segment prefix', () => {
  expect(scopePathWithin(schemaPath, [])).toBe(true)
  expect(scopePathWithin(schemaPath, schemaPath.slice(0, 1))).toBe(true)
  expect(scopePathWithin(schemaPath.slice(0, 1), schemaPath)).toBe(false)
  expect(
    scopePathWithin([{ kind: 'database', name: 'app2' }], [{ kind: 'database', name: 'app' }]),
  ).toBe(false)
})

it('writes refreshed listings and prunes unreturned listings under the refreshed path', () => {
  const client = new QueryClient()
  const tablePath = [...schemaPath, { kind: 'table', name: 'gone' }]
  const otherSchema = [schemaPath[0], { kind: 'schema', name: 'sales' }]
  const keyOf = (path: NavigatorListing['path'], folder: string) =>
    queryKeys.connectionSchemaNodes('acme', 3, 7, path, folder)
  client.setQueryData(keyOf(schemaPath, 'tables'), listing(schemaPath, 'tables', ['gone']))
  client.setQueryData(keyOf(tablePath, 'columns'), listing(tablePath, 'columns', ['id']))
  client.setQueryData(keyOf(otherSchema, 'tables'), listing(otherSchema, 'tables', ['kept']))

  applyNavigatorListings(
    client,
    'acme',
    3,
    7,
    [listing(schemaPath, 'tables', ['orders'])],
    schemaPath,
  )

  expect(
    client.getQueryData<NavigatorListing>(keyOf(schemaPath, 'tables'))?.items.map((i) => i.name),
  ).toEqual(['orders'])
  expect(client.getQueryData(keyOf(tablePath, 'columns'))).toBeUndefined()
  expect(client.getQueryData(keyOf(otherSchema, 'tables'))).toBeDefined()
})
