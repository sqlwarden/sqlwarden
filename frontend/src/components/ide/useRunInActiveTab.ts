import { useCallback } from 'react'
import { toast } from 'sonner'
import type { Connection } from '#/lib/api/types'
import { useIde, activeTabId as selectActiveTabId } from './useIdeStore'
import { useQueryRunRegistry } from './useQueryRunRegistry'

export function useRunInActiveTab(workspaceId: number) {
  const activeTabId = useIde((s) => selectActiveTabId(s, workspaceId))
  const registry = useQueryRunRegistry()

  return useCallback(
    (connection: Connection | undefined, sql: string) => {
      if (!connection) {
        toast.warning('The connection for this query is no longer available.')
        return
      }
      const runner = activeTabId ? registry.get(activeTabId) : undefined
      if (!runner) {
        toast.warning('Open a SQL editor tab to run this query.')
        return
      }
      void runner(connection, sql)
    },
    [activeTabId, registry],
  )
}
