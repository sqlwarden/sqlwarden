import { errorMessage, isApiError } from '#/lib/api/errors'
import type { ResultSet, UnsafeStatement } from '#/lib/api/types'
import type { PendingUnsafeQuery, QueryResult, TransactionState } from './useIdeStore'

export type RecordHistoryInput = {
  sqlText: string
  status: 'ok' | 'error' | 'cancelled'
  errorMessage?: string
  durationMs: number
  rowsAffected: number
}

export function isQueryAbort(error: unknown) {
  return error instanceof Error && error.name === 'AbortError'
}

export type BatchExecutionDependencies = {
  ensureSession: <T>(
    connectionId: number,
    run: (sessionId: string) => Promise<T>,
    signal?: AbortSignal,
  ) => Promise<T>
  runStatement: (
    sessionId: string,
    sql: string,
    confirmUnsafe: boolean,
    signal: AbortSignal,
  ) => Promise<ResultSet>
  beginRun: (tabId: string, sqls: string[]) => string
  setRunStatementResult: (tabId: string, runId: string, index: number, result: QueryResult) => void
  markRunRemainingSkipped: (tabId: string, runId: string, fromIndex: number) => void
  setRunning: (tabId: string, running: boolean) => void
  setController: (tabId: string, controller: AbortController | null) => void
  setPendingConfirmation: (tabId: string, pending: PendingUnsafeQuery) => void
  recordHistory: (entry: RecordHistoryInput) => void
  setTransactionState: (connectionId: number, state: TransactionState) => void
}

export type BatchExecutionRequest = {
  tabId: string
  connectionId: number
  sqls: string[]
  controller: AbortController
  /** An existing run to resume into (e.g. from confirmAt). Omit to start a new run. */
  runId?: string
  startAt?: number
  confirmUnsafeAt?: number
}

const UNSAFE_QUERY_CONFIRMATION_REQUIRED = 'unsafe_query_confirmation_required'

/** Runs every statement in `request.sqls` sequentially over one session as one
 *  run, stopping on the first error, cancel, or unconfirmed unsafe statement.
 *  Resolves to the run's id. */
export async function runStatementBatch(
  request: BatchExecutionRequest,
  deps: BatchExecutionDependencies,
): Promise<string> {
  const { tabId, connectionId, sqls, controller, confirmUnsafeAt } = request

  const runId = request.runId ?? deps.beginRun(tabId, sqls)
  const startAt = request.startAt ?? 0

  deps.setController(tabId, controller)
  deps.setRunning(tabId, true)

  try {
    for (let i = startAt; i < sqls.length; i++) {
      const sql = sqls[i]
      deps.setRunStatementResult(tabId, runId, i, { status: 'running', sql })

      try {
        const result = await deps.ensureSession(
          connectionId,
          (sessionId) =>
            deps.runStatement(sessionId, sql, i === confirmUnsafeAt, controller.signal),
          controller.signal,
        )
        deps.setRunStatementResult(tabId, runId, i, {
          status: 'ok',
          data: result,
          durationMs: result.duration_ms,
          sql,
          connectionId,
        })
        deps.setTransactionState(connectionId, {
          mode: result.transaction.mode,
          open: result.transaction.open,
          pendingStatements: result.transaction.pending_statements,
        })
        deps.recordHistory({
          sqlText: sql,
          status: 'ok',
          durationMs: result.duration_ms,
          rowsAffected: result.rows_affected ?? 0,
        })
      } catch (error) {
        if (isApiError(error) && error.code === UNSAFE_QUERY_CONFIRMATION_REQUIRED) {
          deps.setRunStatementResult(tabId, runId, i, { status: 'pending', sql })
          deps.setPendingConfirmation(tabId, {
            sql,
            statements: (error.details as UnsafeStatement[] | undefined) ?? [],
            runId,
            statementIndex: i,
          })
          return runId
        }
        if (isQueryAbort(error)) {
          deps.setRunStatementResult(tabId, runId, i, { status: 'cancelled', sql })
          deps.recordHistory({ sqlText: sql, status: 'cancelled', durationMs: 0, rowsAffected: 0 })
          deps.markRunRemainingSkipped(tabId, runId, i + 1)
          return runId
        }
        const message = errorMessage(error, 'Query failed')
        deps.setRunStatementResult(tabId, runId, i, { status: 'error', message, sql })
        deps.recordHistory({
          sqlText: sql,
          status: 'error',
          errorMessage: message,
          durationMs: 0,
          rowsAffected: 0,
        })
        deps.markRunRemainingSkipped(tabId, runId, i + 1)
        return runId
      }
    }
  } finally {
    deps.setController(tabId, null)
    deps.setRunning(tabId, false)
  }

  return runId
}
