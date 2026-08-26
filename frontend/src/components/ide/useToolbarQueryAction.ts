import { useCallback, useEffect } from 'react'
import { toast } from 'sonner'
import type { Connection, Workspace } from '#/lib/api/types'
import { sqlStatementAtCursor } from './sqlStatements'
import type { EditorTab } from './useIdeStore'
import { useIde } from './useIdeStore'
import { useEditorViewRegistry, type EditorViewRegistry } from './useEditorViewRegistry'
import { useQueryExecution } from './useQueryExecution'
import { useRunAllStatements } from './useRunAllStatements'
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

export function resolveEditorDocumentText({
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
  if (view) return view.state.doc.toString().trim()
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
  bindShortcut = true,
}: {
  orgSlug: string
  workspace: Workspace
  activeTab?: EditorTab
  activeGroupId?: string
  activeConnection?: Connection
  hasConnections: boolean
  /** Registers the Ctrl/Cmd+Enter run shortcut. A workspace has exactly one
   *  "global" active tab, so only one caller (the toolbar) should bind it —
   *  callers scoped to a single split pane (e.g. an editor context menu) pass
   *  false to avoid double-registering the listener. */
  bindShortcut?: boolean
}) {
  const documentRegistry = useYDocRegistry()
  const viewRegistry = useEditorViewRegistry()
  const maximizedPane = useIde((state) => state.maximizedPane)
  const setMaximizedPane = useIde((state) => state.setMaximizedPane)
  const {
    cancel: cancelRun,
    confirmAt: confirmAtPlain,
    isRunning: isRunningPlain,
    run: execute,
  } = useQueryExecution(orgSlug, workspace.id, activeTab?.id, activeConnection?.id)
  const {
    runAll: executeAll,
    confirmAt: confirmAtBatch,
    cancel: cancelBatch,
    isRunning: isRunningBatch,
  } = useRunAllStatements(orgSlug, workspace.id, activeTab?.id, activeConnection?.id)

  const isRunning = isRunningPlain || isRunningBatch

  const cancel = useCallback(() => {
    cancelRun()
    cancelBatch()
  }, [cancelRun, cancelBatch])

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

  const resolveDocumentText = useCallback(
    () =>
      resolveEditorDocumentText({
        activeGroupId,
        tab: activeTab,
        viewRegistry,
        documentRegistry,
      }),
    [activeGroupId, activeTab, viewRegistry, documentRegistry],
  )

  const run = useCallback(async () => {
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
    await execute(sql)
  }, [
    activeTab,
    activeConnection,
    hasConnections,
    isRunning,
    maximizedPane,
    resolveSql,
    setMaximizedPane,
    execute,
  ])

  const confirmAt = useCallback(
    async (index: number) => {
      await Promise.all([confirmAtPlain(index), confirmAtBatch(index)])
    },
    [confirmAtPlain, confirmAtBatch],
  )

  const runAll = useCallback(
    async (sqls: string[]) => {
      if (!activeTab || isRunning || sqls.length === 0) return
      if (!activeConnection) {
        toast.warning(
          hasConnections
            ? 'Select a connection to run this query.'
            : 'No connection available. Add a connection to run queries.',
        )
        return
      }
      if (maximizedPane === 'editor') setMaximizedPane(null)
      await executeAll(sqls)
    },
    [
      activeTab,
      activeConnection,
      executeAll,
      hasConnections,
      isRunning,
      maximizedPane,
      setMaximizedPane,
    ],
  )

  useEffect(() => {
    if (!bindShortcut) return
    function onKeyDown(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key === 'Enter') {
        event.preventDefault()
        event.stopPropagation()
        void run()
      }
    }
    window.addEventListener('keydown', onKeyDown, { capture: true })
    return () => window.removeEventListener('keydown', onKeyDown, { capture: true })
  }, [run, bindShortcut])

  return { cancel, confirmAt, isRunning, resolveDocumentText, resolveSql, run, runAll }
}
