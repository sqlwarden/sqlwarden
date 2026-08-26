import { closeConnectionQueryCursor } from '#/lib/api/query'
import type { ResultRun } from './useIdeStore'

/** Max runs kept per editor tab; the oldest run is evicted past this. */
export const RUN_HISTORY_CAP = 50

/** Returns the oldest unpinned runs that exceed `RUN_HISTORY_CAP`, oldest
 *  first. Pinned runs don't count against the cap and are never evicted. */
export function runsToEvict(existing: ResultRun[]): ResultRun[] {
  const unpinned = existing.filter((r) => !r.pinned)
  const excess = unpinned.length - RUN_HISTORY_CAP
  return excess > 0 ? unpinned.slice(0, excess) : []
}

/** Closes every open server-side query cursor an evicted run was holding, so
 *  eviction doesn't leak backend cursor resources. Best-effort: a cursor that
 *  already expired or failed to close is not worth surfacing to the user. */
export async function closeRunCursors(
  orgSlug: string,
  workspaceId: number,
  fallbackConnectionId: number | undefined,
  run: ResultRun,
): Promise<void> {
  for (const result of run.results) {
    if (result.status !== 'ok') continue
    const cursorId = result.data.query_cursor_id
    if (!cursorId) continue
    const connectionId = result.connectionId ?? fallbackConnectionId
    if (!connectionId) continue
    await closeConnectionQueryCursor(orgSlug, workspaceId, connectionId, cursorId).catch(
      () => undefined,
    )
  }
}
