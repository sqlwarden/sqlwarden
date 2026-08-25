import { useCallback, useRef } from 'react'
import { runConnectionQuery } from '#/lib/api/query'
import { runStatementBatch, type BatchExecutionDependencies } from './runStatementBatch'
import { useEnsureSession } from './sessionErrors'
import { useRefreshTransactionState } from './transactionState'
import { useBeginRun } from './useBeginRun'
import { useHistoryRecorder } from './useHistoryRecorder'
import { useIde } from './useIdeStore'

export function useQueryExecution(
  orgSlug: string,
  workspaceId: number,
  tabId: string | undefined,
  connectionId: number | undefined,
) {
  const ensureSession = useEnsureSession(orgSlug, workspaceId)
  const recordHistory = useHistoryRecorder(orgSlug, workspaceId, connectionId ?? 0)
  const beginRun = useBeginRun(orgSlug, workspaceId, tabId, connectionId)
  const previousController = useIde((state) => (tabId ? state.abortControllers[tabId] : undefined))
  const isRunning = useIde((state) => Boolean(tabId && state.runningTabs[tabId]))
  const pendingRunId = useIde((state) =>
    tabId ? state.pendingConfirmations[tabId]?.runId : undefined,
  )
  const setRunStatementResult = useIde((state) => state.setRunStatementResult)
  const markRunRemainingSkipped = useIde((state) => state.markRunRemainingSkipped)
  const setRunning = useIde((state) => state.setTabRunning)
  const setController = useIde((state) => state.setTabController)
  const setPendingConfirmation = useIde((state) => state.setPendingConfirmation)
  const setTransactionState = useIde((state) => state.setTransactionState)
  const refreshTransactionState = useRefreshTransactionState(orgSlug, workspaceId)

  const runIdRef = useRef('')
  const sqlRef = useRef('')

  const cancel = useCallback(() => previousController?.abort(), [previousController])

  const executeBatch = useCallback(
    (
      sql: string,
      controller: AbortController,
      options: { runId: string; startAt?: number; confirmUnsafeAt?: number },
    ) => {
      if (!tabId || !connectionId) return Promise.resolve('')
      const deps: BatchExecutionDependencies = {
        ensureSession,
        runStatement: (sessionId, statementSql, confirmUnsafe, signal) =>
          runConnectionQuery(orgSlug, workspaceId, connectionId, sessionId, statementSql, {
            useCursor: true,
            confirmUnsafe,
            signal,
          }),
        beginRun: (_tabId, sqls) => beginRun(sqls),
        setRunStatementResult,
        markRunRemainingSkipped,
        setRunning,
        setController,
        setPendingConfirmation,
        recordHistory,
        setTransactionState,
        refreshTransactionState,
      }
      return runStatementBatch({ tabId, connectionId, sqls: [sql], controller, ...options }, deps)
    },
    [
      beginRun,
      connectionId,
      ensureSession,
      markRunRemainingSkipped,
      orgSlug,
      recordHistory,
      refreshTransactionState,
      setController,
      setPendingConfirmation,
      setRunStatementResult,
      setRunning,
      setTransactionState,
      tabId,
      workspaceId,
    ],
  )

  const run = useCallback(
    async (sql: string) => {
      if (!tabId || !connectionId || isRunning) return

      previousController?.abort()
      sqlRef.current = sql
      const controller = new AbortController()
      const runId = beginRun([sql])
      runIdRef.current = runId
      await executeBatch(sql, controller, { runId })
    },
    [beginRun, connectionId, executeBatch, isRunning, previousController, tabId],
  )

  const confirmAt = useCallback(
    async (index: number) => {
      if (!tabId || !connectionId) return
      if (!runIdRef.current || runIdRef.current !== pendingRunId) return

      const controller = new AbortController()
      await executeBatch(sqlRef.current, controller, {
        runId: runIdRef.current,
        startAt: index,
        confirmUnsafeAt: index,
      })
    },
    [connectionId, executeBatch, pendingRunId, tabId],
  )

  return { cancel, confirmAt, isRunning, run }
}
