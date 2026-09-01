import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import {
  clearCompletionIndexCache,
  getCompletionIndex,
  invalidateCompletionIndex,
} from './schemaIndex'

const config = {
  orgSlug: 'acme',
  workspaceId: 3,
  connectionId: 7,
  sessionId: 's1',
  driver: 'postgres',
}

function indexResponse() {
  return new Response(
    JSON.stringify({
      version: 'snap-1',
      default_schema: 'public',
      schemas: ['public'],
      objects: [{ schema: 'public', name: 'orders', kind: 'table' }],
      columns: [
        { schema: 'public', table: 'orders', name: 'id', type: 'int8', nullable: false },
        { schema: 'public', table: 'orders', name: 'total', type: 'numeric', nullable: true },
      ],
    }),
    { status: 200, headers: { 'Content-Type': 'application/json' } },
  )
}

beforeEach(() => clearCompletionIndexCache())
afterEach(() => vi.unstubAllGlobals())

it('fetches once per connection per session and memoizes the result', async () => {
  const fetchMock = vi.fn(async () => indexResponse())
  vi.stubGlobal('fetch', fetchMock)

  const a = await getCompletionIndex(config)
  const b = await getCompletionIndex(config)

  expect(fetchMock).toHaveBeenCalledTimes(1)
  expect(a).toBe(b)
  expect(a?.defaultSchema).toBe('public')
  expect(a?.allColumns).toHaveLength(2)
  expect(a?.columnsByTable.get('public orders')?.map((c) => c.name)).toEqual(['id', 'total'])
  expect(a?.columnsByTable.get('orders')?.map((c) => c.name)).toEqual(['id', 'total'])
})

it('refetches after invalidateCompletionIndex', async () => {
  const fetchMock = vi.fn(async () => indexResponse())
  vi.stubGlobal('fetch', fetchMock)

  await getCompletionIndex(config)
  invalidateCompletionIndex(7)
  await getCompletionIndex(config)

  expect(fetchMock).toHaveBeenCalledTimes(2)
})

it('resolves to null on failure and does not retry within the backoff window', async () => {
  const fetchMock = vi.fn(async () => new Response('nope', { status: 500 }))
  vi.stubGlobal('fetch', fetchMock)

  expect(await getCompletionIndex(config)).toBeNull()
  expect(await getCompletionIndex(config)).toBeNull()
  expect(fetchMock).toHaveBeenCalledTimes(1)
})

it('returns null when the connection is not fully identified', async () => {
  const fetchMock = vi.fn(async () => indexResponse())
  vi.stubGlobal('fetch', fetchMock)

  expect(await getCompletionIndex({ driver: 'postgres' })).toBeNull()
  expect(fetchMock).not.toHaveBeenCalled()
})
