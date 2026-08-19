import { useCallback, useEffect } from 'react'
import { toast } from 'sonner'
import type { Connection, Workspace } from '#/lib/api/types'
import { sqlStatementAtCursor } from './sqlStatements'
import type { EditorTab } from './useIdeStore'
import { useIde } from './useIdeStore'
import { useEditorViewRegistry, type EditorViewRegistry } from './useEditorViewRegistry'
import { useQueryExecution } from './useQueryExecution'
import { useYDocRegistry, type YDocRegistry } from './useYDocRegistry'

export function resolveEditorSql({
  activeGroupId,
  tab,
  viewRegistry,
  documentRegistry,
}: {
  activeGroupId?: string
  tab?: EditorTab
  viewRegistry: EditorViewRegistry
  documentRegistry: YDocRegistry
}): string {
  if (!tab) return ''
  const view = viewRegistry.get(activeGroupId ? `${activeGroupId}:${tab.id}` : tab.id)
  if (view) {
    const selection = view.state.selection.main
    if (selection.from !== selection.to)
      return view.state.sliceDoc(selection.from, selection.to).trim()
    return sqlStatementAtCursor(view.state.doc.toString(), selection.head)
  }
  const document = documentRegistry.get(tab.id)
  return (document ? document.getText('content').toString() : tab.content).trim()
}

export function useToolbarQueryAction({
  orgSlug,
  workspace,
  activeTab,
  activeGroupId,
  activeConnection,
  hasConnections,
}: {
  orgSlug: string
  workspace: Workspace
  activeTab?: EditorTab
  activeGroupId?: string
  activeConnection?: Connection
  hasConnections: boolean
}) {
  const documentRegistry = useYDocRegistry()
  const viewRegistry = useEditorViewRegistry()
  const maximizedPane = useIde((state) => state.maximizedPane)
  const setMaximizedPane = useIde((state) => state.setMaximizedPane)
  const {
    cancel,
    isRunning,
    run: execute,
  } = useQueryExecution(orgSlug, workspace.id, activeTab?.id, activeConnection?.id)

  const resolveSql = useCallback(
    () =>
      resolveEditorSql({
        activeGroupId,
        tab: activeTab,
        viewRegistry,
        documentRegistry,
      }),
    [activeGroupId, activeTab, viewRegistry, documentRegistry],
  )

  const run = useCallback(
    async (confirmUnsafe?: boolean) => {
      if (!activeTab || isRunning) return
      if (!activeConnection) {
        toast.warning(
          hasConnections
            ? 'Select a connection to run this query.'
            : 'No connection available. Add a connection to run queries.',
        )
        return
      }
      const sql = resolveSql()
      if (!sql) return
      if (maximizedPane === 'editor') setMaximizedPane(null)
      await execute(sql, confirmUnsafe)
    },
    [
      activeTab,
      activeConnection,
      hasConnections,
      isRunning,
      maximizedPane,
      resolveSql,
      setMaximizedPane,
      execute,
    ],
  )

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
        event.preventDefault()
        event.stopPropagation()
        void run()
      }
    }
    window.addEventListener('keydown', onKeyDown, { capture: true })
    return () => window.removeEventListener('keydown', onKeyDown, { capture: true })
  }, [run])

  return { cancel, isRunning, resolveSql, run }
}
