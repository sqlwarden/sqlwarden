import { useCallback, useContext } from 'react'
import { closeRunCursors, runsToEvict } from './resultRunHistory'
import { IdeStoreContext } from './useIdeStore'

/** Starts a new run for a tab and evicts+closes the oldest run's cursors once
 *  the tab's history exceeds the cap. Reads the store directly (not via a
 *  subscribed selector) so eviction sees the just-appended run synchronously. */
export function useBeginRun(
  orgSlug: string,
  workspaceId: number,
  tabId: string | undefined,
  fallbackConnectionId: number | undefined,
) {
  const store = useContext(IdeStoreContext)

  return useCallback(
    (sqls: string[]): string => {
      if (!store || !tabId) return ''
      const s = store.getState()
      const runId = s.beginRun(tabId, sqls, fallbackConnectionId)

      const evicted = runsToEvict(store.getState().resultRuns[tabId] ?? [])
      if (evicted.length > 0) {
        store.getState().evictRuns(
          tabId,
          evicted.map((run) => run.id),
        )
        for (const run of evicted) {
          void closeRunCursors(orgSlug, workspaceId, fallbackConnectionId, run)
        }
      }

      return runId
    },
    [fallbackConnectionId, orgSlug, store, tabId, workspaceId],
  )
}
