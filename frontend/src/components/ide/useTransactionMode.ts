import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  commitConnectionTransaction,
  rollbackConnectionTransaction,
  setConnectionTransactionMode,
} from '#/lib/api/queries/database'
import { errorMessage } from '#/lib/api/errors'
import { useIde, type TransactionState } from './useIdeStore'

const DEFAULT_STATE: TransactionState = { mode: 'auto', open: false, pendingStatements: 0 }

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
  const queryClient = useQueryClient()

  const modeMutation = useMutation({
    mutationFn: (mode: 'auto' | 'manual') => {
      if (!connectionId || !sessionId) throw new Error('No active session')
      return setConnectionTransactionMode(orgSlug, workspaceId, connectionId, sessionId, mode)
    },
    onSuccess: (data) => {
      if (!connectionId) return
      if (data.mode === 'manual' && state.mode !== 'manual') {
        toast.info(
          "Manual commit mode is on. Changes won't be saved until you commit — some DDL may still commit immediately depending on the connected engine.",
        )
      }
      setTransactionState(connectionId, {
        mode: data.mode,
        open: data.open,
        pendingStatements: data.pending_statements,
      })
    },
    onError: (error) => toast.error(errorMessage(error, 'Failed to change transaction mode')),
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
      })
      void queryClient.invalidateQueries()
    },
    onError: (error) => toast.error(errorMessage(error, 'Failed to commit transaction')),
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
      })
    },
    onError: (error) => toast.error(errorMessage(error, 'Failed to roll back transaction')),
  })

  function switchToManual() {
    modeMutation.mutate('manual')
  }

  /** Returns 'blocked' without mutating if a transaction is open — the caller
   *  (TransactionControls) shows the Commit/Rollback/Cancel guard dialog in
   *  that case instead of calling this again. */
  async function switchToAuto(): Promise<'ok' | 'blocked'> {
    if (state.open) return 'blocked'
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
