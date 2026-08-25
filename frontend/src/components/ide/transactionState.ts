import { useCallback, useEffect } from 'react'
import { getConnectionTransactionStatus } from '#/lib/api/queries/database'
import type { TransactionStatusResponse } from '#/lib/api/types'
import { useIdeStoreApi, type TransactionState } from './useIdeStore'

export const DEFAULT_TRANSACTION_STATE: TransactionState = {
  mode: 'auto',
  open: false,
  pendingStatements: 0,
  statements: [],
}

export function toTransactionState(response: TransactionStatusResponse): TransactionState {
  return {
    mode: response.mode,
    open: response.open,
    pendingStatements: response.pending_statements,
    statements: response.statements,
  }
}

/**
 * Refreshes a connection's transaction state from the backend, the source of
 * truth for whether a transaction is open. Needed wherever local state can
 * drift: a failed statement (the backend opened the transaction before the
 * statement failed), a cancellation (which destroys the session server-side),
 * and hydration of a newly active session.
 *
 * A missing or dead session cannot have an open transaction, so both reset the
 * connection to the auto-commit default rather than leaving stale pending
 * counts on screen.
 */
export function useRefreshTransactionState(orgSlug: string, workspaceId: number) {
  const store = useIdeStoreApi()

  return useCallback(
    async (connectionId: number) => {
      const { sessions, setTransactionState } = store.getState()
      const sessionId = sessions[connectionId]
      if (!sessionId) {
        setTransactionState(connectionId, DEFAULT_TRANSACTION_STATE)
        return
      }
      try {
        const status = await getConnectionTransactionStatus(
          orgSlug,
          workspaceId,
          connectionId,
          sessionId,
        )
        store.getState().setTransactionState(connectionId, toTransactionState(status))
      } catch {
        store.getState().setTransactionState(connectionId, DEFAULT_TRANSACTION_STATE)
      }
    },
    [orgSlug, workspaceId, store],
  )
}

/**
 * Hydrates transaction state whenever a session becomes active for a
 * connection — tab mount, switching to a tab on an already-connected
 * connection, and reconnect (a new session id). Without this the store would
 * show auto-commit defaults for a connection another tab or a previous page
 * load left in manual mode with an open transaction.
 */
export function useTransactionHydration(
  orgSlug: string,
  workspaceId: number,
  connectionId: number | undefined,
  sessionId: string | undefined,
) {
  const refresh = useRefreshTransactionState(orgSlug, workspaceId)

  useEffect(() => {
    if (!connectionId || !sessionId) return
    void refresh(connectionId)
  }, [connectionId, sessionId, refresh])
}
