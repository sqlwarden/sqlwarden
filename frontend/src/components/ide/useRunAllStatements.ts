import { useCallback, useRef } from 'react'
import { runConnectionQuery } from '#/lib/api/query'
import { runStatementBatch, type BatchExecutionDependencies } from './runStatementBatch'
import { useEnsureSession } from './sessionErrors'
import { useBeginRun } from './useBeginRun'
import { useHistoryRecorder } from './useHistoryRecorder'
import { useIde } from './useIdeStore'

export function useRunAllStatements(
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

  const sqlsRef = useRef<string[]>([])
  const runIdRef = useRef('')

  const cancel = useCallback(() => previousController?.abort(), [previousController])

  const executeBatch = useCallback(
    (
      sqls: string[],
      controller: AbortController,
      options: { runId: string; startAt?: number; confirmUnsafeAt?: number },
    ) => {
      if (!tabId || !connectionId) return Promise.resolve('')
      const deps: BatchExecutionDependencies = {
        ensureSession,
        runStatement: (sessionId, sql, confirmUnsafe, signal) =>
          runConnectionQuery(orgSlug, workspaceId, connectionId, sessionId, sql, {
            useCursor: true,
            confirmUnsafe,
            signal,
          }),
        beginRun: (_tabId, batchSqls) => beginRun(batchSqls),
        setRunStatementResult,
        markRunRemainingSkipped,
        setRunning,
        setController,
        setPendingConfirmation,
        recordHistory,
        setTransactionState,
      }
      return runStatementBatch({ tabId, connectionId, sqls, controller, ...options }, deps)
    },
    [
      beginRun,
      connectionId,
      ensureSession,
      markRunRemainingSkipped,
      orgSlug,
      recordHistory,
      setController,
      setPendingConfirmation,
      setRunStatementResult,
      setRunning,
      setTransactionState,
      tabId,
      workspaceId,
    ],
  )

  const runAll = useCallback(
    async (sqls: string[]) => {
      if (!tabId || !connectionId || isRunning || sqls.length === 0) return

      previousController?.abort()
      sqlsRef.current = sqls

      const controller = new AbortController()
      const runId = beginRun(sqls)
      runIdRef.current = runId
      await executeBatch(sqls, controller, { runId })
    },
    [beginRun, connectionId, executeBatch, isRunning, previousController, tabId],
  )

  const confirmAt = useCallback(
    async (index: number) => {
      if (!tabId || !connectionId) return
      if (!runIdRef.current || runIdRef.current !== pendingRunId) return
      const sqls = sqlsRef.current
      if (sqls.length === 0) return

      const controller = new AbortController()
      await executeBatch(sqls, controller, {
        runId: runIdRef.current,
        startAt: index,
        confirmUnsafeAt: index,
      })
    },
    [connectionId, executeBatch, pendingRunId, tabId],
  )

  return { runAll, confirmAt, cancel, isRunning }
}
