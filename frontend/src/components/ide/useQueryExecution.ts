import { useCallback } from 'react'
import { closeConnectionQueryCursor, runConnectionQuery } from '#/lib/api/query'
import { executeQuery } from './queryExecution'
import { useEnsureSession } from './sessionErrors'
import { useIde } from './useIdeStore'

export function useQueryExecution(
  orgSlug: string,
  workspaceId: number,
  tabId: string | undefined,
  connectionId: number | undefined,
) {
  const ensureSession = useEnsureSession(orgSlug, workspaceId)
  const previousResult = useIde((state) => (tabId ? state.results[tabId] : undefined))
  const previousController = useIde((state) => (tabId ? state.abortControllers[tabId] : undefined))
  const isRunning = useIde((state) => Boolean(tabId && state.runningTabs[tabId]))
  const setResult = useIde((state) => state.setQueryResult)
  const setRunning = useIde((state) => state.setTabRunning)
  const setController = useIde((state) => state.setTabController)

  const cancel = useCallback(() => previousController?.abort(), [previousController])

  const run = useCallback(
    async (sql: string) => {
      if (!tabId || !connectionId || isRunning) return

      previousController?.abort()
      if (previousResult?.status === 'ok' && previousResult.data.query_cursor_id) {
        const previousConnectionId = previousResult.connectionId ?? connectionId
        void closeConnectionQueryCursor(
          orgSlug,
          workspaceId,
          previousConnectionId,
          previousResult.data.query_cursor_id,
        ).catch(() => undefined)
      }

      const controller = new AbortController()
      await executeQuery(
        { tabId, connectionId, sql, controller },
        {
          ensureSession,
          runQuery: (sessionId, signal) =>
            runConnectionQuery(orgSlug, workspaceId, connectionId, sessionId, sql, {
              useCursor: true,
              signal,
            }),
          setResult,
          setRunning,
          setController,
        },
      )
    },
    [
      connectionId,
      ensureSession,
      isRunning,
      orgSlug,
      previousController,
      previousResult,
      setController,
      setResult,
      setRunning,
      tabId,
      workspaceId,
    ],
  )

  return { cancel, isRunning, run }
}
