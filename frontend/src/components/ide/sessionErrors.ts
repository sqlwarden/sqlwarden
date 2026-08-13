import { useCallback, useEffect } from 'react'
import { api } from '#/lib/api/client'
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

export interface EnsureSessionDeps {
  getSession: (connectionId: number) => string | undefined
  setSession: (connectionId: number, sessionId: string) => void
  clearSession: (connectionId: number) => void
  setConnectionStatus: (
    connectionId: number,
    status: 'connecting' | { error: string } | null,
  ) => void
  connect: (connectionId: number, signal?: AbortSignal) => Promise<string>
}

/**
 * Resolves a live session id for `connectionId`, connecting if none is cached,
 * then calls `run(sessionId)`. If `run` fails because the cached session died
 * server-side (410), clears it, reconnects once, and retries `run` — the same
 * transparent-reconnect behavior Run has always had, now shared with any other
 * caller that needs a live session (e.g. Download Now).
 */
export async function ensureSession<T>(
  deps: EnsureSessionDeps,
  connectionId: number,
  run: (sessionId: string) => Promise<T>,
  signal?: AbortSignal,
): Promise<T> {
  let sessionId = deps.getSession(connectionId)
  for (let attempt = 0; ; attempt++) {
    if (!sessionId) {
      deps.setConnectionStatus(connectionId, 'connecting')
      try {
        sessionId = await deps.connect(connectionId, signal)
        deps.setSession(connectionId, sessionId)
      } finally {
        deps.setConnectionStatus(connectionId, null)
      }
    }
    try {
      return await run(sessionId)
    } catch (err) {
      if (attempt === 0 && isSessionGone(err)) {
        deps.clearSession(connectionId)
        sessionId = undefined
        continue
      }
      throw err
    }
  }
}

/** Wires ensureSession's dependencies to the editor store and the connect endpoint. */
export function useEnsureSession(orgSlug: string, workspaceId: number) {
  const sessions = useIde((s) => s.sessions)
  const setSession = useIde((s) => s.setSession)
  const clearSession = useIde((s) => s.clearSession)
  const setConnectionStatus = useIde((s) => s.setConnectionStatus)

  return useCallback(
    <T>(connectionId: number, run: (sessionId: string) => Promise<T>, signal?: AbortSignal) =>
      ensureSession(
        {
          getSession: (id) => sessions[id],
          setSession,
          clearSession,
          setConnectionStatus,
          connect: async (id, connectSignal) => {
            const data = await api.post<{ session_id: string; reused: boolean }>(
              `/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/connections/${id}/connect`,
              undefined,
              { signal: connectSignal },
            )
            return data.session_id
          },
        },
        connectionId,
        run,
        signal,
      ),
    [sessions, setSession, clearSession, setConnectionStatus, orgSlug, workspaceId],
  )
}
