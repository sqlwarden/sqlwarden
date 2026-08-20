import { errorMessage, isApiError } from '#/lib/api/errors'
import type { ResultSet, UnsafeStatement } from '#/lib/api/types'
import type { PendingUnsafeQuery, QueryResult } from './useIdeStore'

export type QueryExecutionRequest = {
  tabId: string
  connectionId: number
  sql: string
  controller: AbortController
}

export type RecordHistoryInput = {
  sqlText: string
  status: 'ok' | 'error' | 'cancelled'
  errorMessage?: string
  durationMs: number
  rowsAffected: number
}

export type QueryExecutionDependencies = {
  ensureSession: <T>(
    connectionId: number,
    run: (sessionId: string) => Promise<T>,
    signal?: AbortSignal,
  ) => Promise<T>
  runQuery: (sessionId: string, signal: AbortSignal) => Promise<ResultSet>
  setResult: (tabId: string, result: QueryResult) => void
  setRunning: (tabId: string, running: boolean) => void
  setController: (tabId: string, controller: AbortController | null) => void
  setPendingConfirmation: (tabId: string, pending: PendingUnsafeQuery) => void
  recordHistory: (entry: RecordHistoryInput) => void
}

export function isQueryAbort(error: unknown) {
  return error instanceof Error && error.name === 'AbortError'
}

const UNSAFE_QUERY_CONFIRMATION_REQUIRED = 'unsafe_query_confirmation_required'

/** Executes one query and owns every transient store transition for its tab. */
export async function executeQuery(
  request: QueryExecutionRequest,
  dependencies: QueryExecutionDependencies,
) {
  const { connectionId, controller, sql, tabId } = request
  dependencies.setController(tabId, controller)
  dependencies.setRunning(tabId, true)
  dependencies.setResult(tabId, { status: 'running' })

  try {
    const result = await dependencies.ensureSession(
      connectionId,
      (sessionId) => dependencies.runQuery(sessionId, controller.signal),
      controller.signal,
    )
    dependencies.setResult(tabId, {
      status: 'ok',
      data: result,
      durationMs: result.duration_ms,
      sql,
      connectionId,
    })
    dependencies.recordHistory({
      sqlText: sql,
      status: 'ok',
      durationMs: result.duration_ms,
      rowsAffected: result.rows_affected ?? 0,
    })
  } catch (error) {
    if (isApiError(error) && error.code === UNSAFE_QUERY_CONFIRMATION_REQUIRED) {
      dependencies.setPendingConfirmation(tabId, {
        sql,
        statements: (error.details as UnsafeStatement[] | undefined) ?? [],
      })
    } else if (isQueryAbort(error)) {
      dependencies.setResult(tabId, { status: 'cancelled', sql })
      dependencies.recordHistory({
        sqlText: sql,
        status: 'cancelled',
        durationMs: 0,
        rowsAffected: 0,
      })
    } else {
      const message = errorMessage(error, 'Query failed')
      dependencies.setResult(tabId, { status: 'error', message, sql })
      dependencies.recordHistory({
        sqlText: sql,
        status: 'error',
        errorMessage: message,
        durationMs: 0,
        rowsAffected: 0,
      })
    }
  } finally {
    dependencies.setController(tabId, null)
    dependencies.setRunning(tabId, false)
  }
}
