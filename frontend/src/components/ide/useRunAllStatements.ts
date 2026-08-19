import { useCallback, useRef } from 'react'
import { runConnectionQuery } from '#/lib/api/query'
import { runStatementBatch } from './runStatementBatch'
import { useEnsureSession } from './sessionErrors'
import { useIde } from './useIdeStore'

export function useRunAllStatements(
  orgSlug: string,
  workspaceId: number,
  tabId: string | undefined,
  connectionId: number | undefined,
) {
  const ensureSession = useEnsureSession(orgSlug, workspaceId)
  const previousController = useIde((state) => (tabId ? state.abortControllers[tabId] : undefined))
  const isRunning = useIde((state) => Boolean(tabId && state.runningTabs[tabId]))
  const initBatch = useIde((state) => state.initBatchResults)
  const setStatementResult = useIde((state) => state.setStatementResult)
  const markRemainingSkipped = useIde((state) => state.markRemainingSkipped)
  const setRunning = useIde((state) => state.setTabRunning)
  const setController = useIde((state) => state.setTabController)
  const setPendingConfirmation = useIde((state) => state.setPendingConfirmation)

  const sqlsRef = useRef<string[]>([])

  const cancel = useCallback(() => previousController?.abort(), [previousController])

  const runAll = useCallback(
    async (sqls: string[]) => {
      if (!tabId || !connectionId || isRunning || sqls.length === 0) return

      previousController?.abort()
      sqlsRef.current = sqls

      const controller = new AbortController()
      await runStatementBatch(
        { tabId, connectionId, sqls, controller },
        {
          ensureSession,
          runStatement: (sessionId, sql, confirmUnsafe, signal) =>
            runConnectionQuery(orgSlug, workspaceId, connectionId, sessionId, sql, {
              useCursor: true,
              confirmUnsafe,
              signal,
            }),
          initBatch,
          setStatementResult,
          markRemainingSkipped,
          setRunning,
          setController,
          setPendingConfirmation,
        },
      )
    },
    [
      connectionId,
      ensureSession,
      initBatch,
      isRunning,
      markRemainingSkipped,
      orgSlug,
      previousController,
      setController,
      setPendingConfirmation,
      setRunning,
      setStatementResult,
      tabId,
      workspaceId,
    ],
  )

  const confirmAt = useCallback(
    async (index: number) => {
      if (!tabId || !connectionId || isRunning) return
      const sqls = sqlsRef.current
      if (sqls.length === 0) return

      const controller = new AbortController()
      await runStatementBatch(
        { tabId, connectionId, sqls, controller, startAt: index, confirmUnsafeAt: index },
        {
          ensureSession,
          runStatement: (sessionId, sql, confirmUnsafe, signal) =>
            runConnectionQuery(orgSlug, workspaceId, connectionId, sessionId, sql, {
              useCursor: true,
              confirmUnsafe,
              signal,
            }),
          initBatch,
          setStatementResult,
          markRemainingSkipped,
          setRunning,
          setController,
          setPendingConfirmation,
        },
      )
    },
    [
      connectionId,
      ensureSession,
      initBatch,
      isRunning,
      markRemainingSkipped,
      orgSlug,
      setController,
      setPendingConfirmation,
      setRunning,
      setStatementResult,
      tabId,
      workspaceId,
    ],
  )

  return { runAll, confirmAt, cancel, isRunning }
}
