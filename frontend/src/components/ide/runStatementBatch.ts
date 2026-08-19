import { errorMessage, isApiError } from '#/lib/api/errors'
import type { ResultSet, UnsafeStatement } from '#/lib/api/types'
import { isQueryAbort } from './queryExecution'
import type { PendingUnsafeQuery, QueryResult } from './useIdeStore'

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
  initBatch: (tabId: string, sqls: string[]) => void
  setStatementResult: (tabId: string, index: number, result: QueryResult) => void
  markRemainingSkipped: (tabId: string, fromIndex: number) => void
  setRunning: (tabId: string, running: boolean) => void
  setController: (tabId: string, controller: AbortController | null) => void
  setPendingConfirmation: (tabId: string, pending: PendingUnsafeQuery) => void
}

export type BatchExecutionRequest = {
  tabId: string
  connectionId: number
  sqls: string[]
  controller: AbortController
  startAt?: number
  confirmUnsafeAt?: number
}

const UNSAFE_QUERY_CONFIRMATION_REQUIRED = 'unsafe_query_confirmation_required'

/** Runs every statement in `request.sqls` sequentially over one session, stopping on the first error, cancel, or unconfirmed unsafe statement. */
export async function runStatementBatch(
  request: BatchExecutionRequest,
  deps: BatchExecutionDependencies,
): Promise<void> {
  const { tabId, connectionId, sqls, controller, confirmUnsafeAt } = request

  if (request.startAt === undefined) {
    deps.initBatch(tabId, sqls)
  }
  const startAt = request.startAt ?? 0

  deps.setController(tabId, controller)
  deps.setRunning(tabId, true)

  try {
    for (let i = startAt; i < sqls.length; i++) {
      const sql = sqls[i]
      deps.setStatementResult(tabId, i, { status: 'running' })

      try {
        const result = await deps.ensureSession(
          connectionId,
          (sessionId) => deps.runStatement(sessionId, sql, i === confirmUnsafeAt, controller.signal),
          controller.signal,
        )
        deps.setStatementResult(tabId, i, {
          status: 'ok',
          data: result,
          durationMs: result.duration_ms,
          sql,
          connectionId,
        })
      } catch (error) {
        if (isApiError(error) && error.code === UNSAFE_QUERY_CONFIRMATION_REQUIRED) {
          deps.setStatementResult(tabId, i, { status: 'pending', sql })
          deps.setPendingConfirmation(tabId, {
            sql,
            statements: (error.details as UnsafeStatement[] | undefined) ?? [],
            batchIndex: i,
          })
          return
        }
        if (isQueryAbort(error)) {
          deps.setStatementResult(tabId, i, { status: 'cancelled', sql })
          deps.markRemainingSkipped(tabId, i + 1)
          return
        }
        deps.setStatementResult(tabId, i, {
          status: 'error',
          message: errorMessage(error, 'Query failed'),
          sql,
        })
        deps.markRemainingSkipped(tabId, i + 1)
        return
      }
    }
  } finally {
    deps.setController(tabId, null)
    deps.setRunning(tabId, false)
  }
}
