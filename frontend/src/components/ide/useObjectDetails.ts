import { useEffect, useMemo, useRef, useState } from 'react'
import { useQueries } from '@tanstack/react-query'
import { objectRefKey, orgConnectionObjectsQueryOptions } from '#/lib/api/query'
import type { ObjectDetail, ObjectRef } from '#/lib/api/types'

export const OBJECT_DETAIL_CHUNK_SIZE = 50
export const OBJECT_DETAIL_MAX_CONCURRENT_CHUNKS = 3

export interface ObjectDetailEntry {
  detail: ObjectDetail | null
  loading: boolean
}

export interface ObjectDetailsResult {
  byRef: Map<string, ObjectDetailEntry>
  /** One entry per in-flight/settled chunk request, for session-eviction checks. */
  errors: unknown[]
}

/** Fetches detail for many objects without firing one request per object.
 *  Refs are grouped into chunks of `chunkSize`, one POST /objects per chunk,
 *  and only `concurrency` chunks are in flight at once, bounding how many
 *  live-inspection requests hit the target driver simultaneously.
 *
 *  A ref keeps the chunk it was first assigned even as `refs` grows, so
 *  earlier chunks' query keys (and their cache entries) stay stable instead
 *  of shifting on every render. */
export function useObjectDetails(opts: {
  orgSlug: string
  workspaceId: string | number
  connectionId: string | number
  sessionId: string | undefined
  refs: ObjectRef[]
  enabled: boolean
  chunkSize?: number
  concurrency?: number
}): ObjectDetailsResult {
  const {
    orgSlug,
    workspaceId,
    connectionId,
    sessionId,
    refs,
    enabled,
    chunkSize = OBJECT_DETAIL_CHUNK_SIZE,
    concurrency = OBJECT_DETAIL_MAX_CONCURRENT_CHUNKS,
  } = opts

  const chunkAssignments = useRef(new Map<string, number>())
  const assignedCount = useRef(0)

  const chunks = useMemo(() => {
    const assignments = chunkAssignments.current
    for (const ref of refs) {
      const key = objectRefKey(ref)
      if (!assignments.has(key)) {
        assignments.set(key, Math.floor(assignedCount.current / chunkSize))
        assignedCount.current += 1
      }
    }
    const byChunk = new Map<number, ObjectRef[]>()
    for (const ref of refs) {
      const index = assignments.get(objectRefKey(ref))!
      const bucket = byChunk.get(index)
      if (bucket) bucket.push(ref)
      else byChunk.set(index, [ref])
    }
    return [...byChunk.entries()].sort((a, b) => a[0] - b[0]).map(([, chunkRefs]) => chunkRefs)
  }, [refs, chunkSize])

  const [unlocked, setUnlocked] = useState(concurrency)

  const results = useQueries({
    queries: chunks.map((chunkRefs, index) => ({
      ...orgConnectionObjectsQueryOptions(orgSlug, workspaceId, connectionId, sessionId, chunkRefs),
      enabled: enabled && index < unlocked,
    })),
  })

  useEffect(() => {
    if (unlocked >= chunks.length) return
    const active = results.slice(0, unlocked).filter((r) => r.isLoading).length
    if (active < concurrency) setUnlocked((u) => Math.min(u + 1, chunks.length))
  }, [results, unlocked, chunks.length, concurrency])

  return useMemo(() => {
    const map = new Map<string, ObjectDetailEntry>()
    chunks.forEach((chunkRefs, index) => {
      const result = results[index]
      const objects = result?.data?.objects
      const loading = (result?.isLoading || result?.data?.status === 'pending') ?? false
      for (const ref of chunkRefs) {
        const detail = objects?.find((o) => objectRefKey(o.ref) === objectRefKey(ref)) ?? null
        map.set(objectRefKey(ref), { detail, loading })
      }
    })
    return { byRef: map, errors: results.map((r) => r.error) }
  }, [chunks, results])
}
