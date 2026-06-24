import { describe, it, expect, vi } from 'vitest'
import { QueryClient, QueryObserver } from '@tanstack/react-query'
import {
  connectionCatalogQueryKey,
  connectionObjectQueryKey,
  invalidateConnectionSchemaQueries,
} from './query'
import type { ObjectRef } from '#/lib/api/types'

const slug = 'acme'
const workspaceId = 1
const connectionId = 2
const ref: ObjectRef = { namespace: 'public', kind: 'table', name: 'users' }

describe('invalidateConnectionSchemaQueries', () => {
  it('invalidates the catalog and an expanded object detail, leaving other connections untouched', async () => {
    const qc = new QueryClient()

    qc.setQueryData(connectionCatalogQueryKey(slug, workspaceId, connectionId), { catalog: {} })
    qc.setQueryData(connectionObjectQueryKey(slug, workspaceId, connectionId, ref), { ref })
    // A second connection's object detail must survive a refresh of the first.
    const otherConnId = 99
    qc.setQueryData(connectionObjectQueryKey(slug, workspaceId, otherConnId, ref), { ref })

    await invalidateConnectionSchemaQueries(qc, slug, workspaceId, connectionId)

    expect(qc.getQueryState(connectionCatalogQueryKey(slug, workspaceId, connectionId))?.isInvalidated).toBe(true)
    expect(qc.getQueryState(connectionObjectQueryKey(slug, workspaceId, connectionId, ref))?.isInvalidated).toBe(true)
    expect(qc.getQueryState(connectionObjectQueryKey(slug, workspaceId, otherConnId, ref))?.isInvalidated).toBe(false)
  })

  it('refetches an actively-expanded object query when its connection is refreshed', async () => {
    const qc = new QueryClient()
    const objectFn = vi.fn().mockResolvedValue({ ref })

    // An expanded tree node is an active observer on the object-detail query.
    const objectKey = connectionObjectQueryKey(slug, workspaceId, connectionId, ref)
    const observer = new QueryObserver(qc, { queryKey: objectKey, queryFn: objectFn })
    const unsubscribe = observer.subscribe(() => {})
    // Wait for the initial fetch to fully settle, otherwise the invalidation's
    // refetch is deduped against the in-flight fetch.
    await vi.waitFor(() => {
      expect(objectFn).toHaveBeenCalledTimes(1)
      expect(qc.getQueryState(objectKey)?.status).toBe('success')
    })

    await invalidateConnectionSchemaQueries(qc, slug, workspaceId, connectionId)

    // invalidateQueries refetches active observers, so the expanded object is
    // re-fetched alongside the catalog.
    expect(objectFn).toHaveBeenCalledTimes(2)
    unsubscribe()
  })
})
