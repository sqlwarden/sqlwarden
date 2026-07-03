import { useEffect } from 'react'
import { isApiError } from '#/lib/api/errors'
import { useIde } from './useIdeStore'

/**
 * True when the target-database session behind X-Warden-Session is gone —
 * the backend answers 410 for schema, query, and cursor endpoints when the
 * session expired or the server restarted. Cursor-expiry 410s carry their own
 * `query_cursor_unavailable` code and only mean the cursor died, not the
 * session, so they are excluded.
 */
export function isSessionGone(error: unknown): boolean {
  return isApiError(error) && error.status === 410 && error.code !== 'query_cursor_unavailable'
}

/**
 * Drops a connection's stored session as soon as any of the given query errors
 * says the backend session is gone. Clearing the session is what un-sticks the
 * UI: status dots turn off, schema/object/diagram surfaces flip to their
 * "not connected" panes with a reconnect action, and the next run auto-connects
 * instead of reusing a dead session id.
 */
export function useEvictGoneSession(connectionId: number | undefined, errors: unknown[]) {
  const clearSession = useIde((s) => s.clearSession)
  const gone = errors.some(isSessionGone)

  useEffect(() => {
    if (gone && connectionId) clearSession(connectionId)
  }, [gone, connectionId, clearSession])
}
