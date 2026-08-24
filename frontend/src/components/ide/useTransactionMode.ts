import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  commitConnectionTransaction,
  rollbackConnectionTransaction,
  setConnectionTransactionMode,
} from '#/lib/api/queries/database'
import { errorMessage } from '#/lib/api/errors'
import { useRefreshTransactionState } from './transactionState'
import { useIde, useIdeStoreApi, type TransactionState } from './useIdeStore'

const DEFAULT_STATE: TransactionState = {
  mode: 'auto',
  open: false,
  pendingStatements: 0,
  statements: [],
}

export function useTransactionMode(
  orgSlug: string,
  workspaceId: number,
  connectionId: number | undefined,
  sessionId: string | undefined,
) {
  const state = useIde((s) =>
    connectionId ? (s.transactions[connectionId] ?? DEFAULT_STATE) : DEFAULT_STATE,
  )
  const setTransactionState = useIde((s) => s.setTransactionState)
  const store = useIdeStoreApi()
  const queryClient = useQueryClient()
  const refreshTransactionState = useRefreshTransactionState(orgSlug, workspaceId)

  // A failed mode-switch/commit/rollback can mean the connection died — the
  // driver's own tx bookkeeping is gone at that point (see the backend's
  // CommitTransaction/RollbackTransaction), but this mutation's local state
  // never learns that. Without resyncing here, the UI keeps showing an open
  // transaction the user can neither commit nor roll back: every retry hits
  // the same dead connection and fails the same way.
  function resyncAfterError(error: unknown, fallbackMessage: string) {
    toast.error(errorMessage(error, fallbackMessage))
    if (connectionId) void refreshTransactionState(connectionId)
  }

  const modeMutation = useMutation({
    mutationFn: (mode: 'auto' | 'manual') => {
      if (!connectionId || !sessionId) throw new Error('No active session')
      return setConnectionTransactionMode(orgSlug, workspaceId, connectionId, sessionId, mode)
    },
    onSuccess: (data) => {
      if (!connectionId) return
      setTransactionState(connectionId, {
        mode: data.mode,
        open: data.open,
        pendingStatements: data.pending_statements,
        statements: data.statements,
      })
    },
    onError: (error) => resyncAfterError(error, 'Failed to change transaction mode'),
  })

  const commitMutation = useMutation({
    mutationFn: () => {
      if (!connectionId || !sessionId) throw new Error('No active session')
      return commitConnectionTransaction(orgSlug, workspaceId, connectionId, sessionId)
    },
    onSuccess: (data) => {
      if (!connectionId) return
      setTransactionState(connectionId, {
        mode: data.mode,
        open: data.open,
        pendingStatements: data.pending_statements,
        statements: data.statements,
      })
      void queryClient.invalidateQueries()
    },
    onError: (error) => resyncAfterError(error, 'Failed to commit transaction'),
  })

  const rollbackMutation = useMutation({
    mutationFn: () => {
      if (!connectionId || !sessionId) throw new Error('No active session')
      return rollbackConnectionTransaction(orgSlug, workspaceId, connectionId, sessionId)
    },
    onSuccess: (data) => {
      if (!connectionId) return
      setTransactionState(connectionId, {
        mode: data.mode,
        open: data.open,
        pendingStatements: data.pending_statements,
        statements: data.statements,
      })
    },
    onError: (error) => resyncAfterError(error, 'Failed to roll back transaction'),
  })

  function switchToManual() {
    modeMutation.mutate('manual')
  }

  /** Returns 'blocked' without mutating if a transaction is open — the caller
   *  (TransactionControls) shows the Commit/Rollback/Cancel guard dialog in
   *  that case instead of calling this again. Reads live state off the store
   *  rather than the `state` closed over at render time: callers such as the
   *  guard dialog's onCommit/onRollback handlers call commit()/rollback()
   *  immediately before this, and by the time this runs `state` from the
   *  render that created the handler is stale. */
  async function switchToAuto(): Promise<'ok' | 'blocked'> {
    const liveState = connectionId
      ? (store.getState().transactions[connectionId] ?? DEFAULT_STATE)
      : DEFAULT_STATE
    if (liveState.open) return 'blocked'
    await modeMutation.mutateAsync('auto')
    return 'ok'
  }

  async function commit() {
    await commitMutation.mutateAsync()
  }

  async function rollback() {
    await rollbackMutation.mutateAsync()
  }

  return { state, switchToManual, switchToAuto, commit, rollback }
}
