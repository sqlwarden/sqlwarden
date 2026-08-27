import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Icon } from '#/lib/icons'
import { Button } from '#/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import {
  orgEnvironmentsQueryOptions,
  allOrgWorkspaceConnectionsQueryOptions,
} from '#/lib/api/query'
import type { Connection, Workspace, WorkspaceFile } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { useIde, activeTabId as selectActiveTabId, type EditorTab } from './useIdeStore'
import { ExplainAnalyzeConfirmDialog } from './ExplainAnalyzeConfirmDialog'
import { SaveAsDialog } from './SaveAsDialog'
import { SaveFavoriteDialog } from './SaveFavoriteDialog'
import { ExportConfirmDialog } from './exports/ExportConfirmDialog'
import { ExportToFilesDialog } from './exports/ExportToFilesDialog'
import { formatBytes } from './exports/formatBytes'
import { useDownloadNow } from './exports/useDownloadNow'
import { Tip } from './schema-diagram/Tip'
import { useSaveEditorTab } from './useSaveEditorTab'
import { ConnectionSelector } from './ConnectionSelector'
import { TransactionControls } from './TransactionControls'
import { TransactionGuardDialog } from './TransactionGuardDialog'
import { useTransactionHydration } from './transactionState'
import { useToolbarQueryAction } from './useToolbarQueryAction'
import { useTransactionMode } from './useTransactionMode'
import { UnsafeQueryDialog } from './UnsafeQueryDialog'
import { useEditorViewRegistry } from './useEditorViewRegistry'
import { formatEditorSql, sqlFormatterForDriver } from './sqlFormatting'
import { isSqlEditorTab, splitSqlStatements } from './sqlStatements'

type IdeToolbarProps = {
  orgSlug: string
  workspace: Workspace
  selection?: string
}

export const RUN_SHORTCUT =
  typeof navigator !== 'undefined' && /mac/i.test(navigator.platform) ? '⌘↵' : 'Ctrl ↵'
export const FORMAT_SHORTCUT =
  typeof navigator !== 'undefined' && /mac/i.test(navigator.platform) ? '⇧⌥F' : 'Shift Alt F'

export function IdeToolbar({ orgSlug, workspace, selection }: IdeToolbarProps) {
  const [saveAsTab, setSaveAsTab] = useState<EditorTab | null>(null)
  const [confirmExportSql, setConfirmExportSql] = useState<string | null>(null)
  const [exportToWorkspaceOpen, setExportToWorkspaceOpen] = useState(false)
  const [saveFavoriteOpen, setSaveFavoriteOpen] = useState(false)
  const [saveFavoriteSql, setSaveFavoriteSql] = useState('')
  const [switchToAutoGuardOpen, setSwitchToAutoGuardOpen] = useState(false)
  const viewRegistry = useEditorViewRegistry()

  const activeTabId = useIde((s) => selectActiveTabId(s, workspace.id))
  const activeGroupId = useIde((s) => s.activeGroupId[workspace.id])
  const tabs = useIde((s) => s.tabs)
  const openTab = useIde((s) => s.openTab)
  const closeTab = useIde((s) => s.closeTab)
  const setTabConnection = useIde((s) => s.setTabConnection)
  const maximizedPane = useIde((s) => s.maximizedPane)
  const setMaximizedPane = useIde((s) => s.setMaximizedPane)
  const clearPendingConfirmation = useIde((s) => s.clearPendingConfirmation)
  const abandonPendingRunConfirmation = useIde((s) => s.abandonPendingRunConfirmation)
  const pendingConfirmation = useIde((s) =>
    activeTabId ? s.pendingConfirmations[activeTabId] : undefined,
  )

  const saveEditorTab = useSaveEditorTab(orgSlug, workspace.id)
  const activeTab = tabs.find((t) => t.id === activeTabId)

  // Export only makes sense for a runnable SQL query — not for a database
  // object/diagram tab or a non-SQL file (e.g. a previously exported CSV).
  const isSqlTab = isSqlEditorTab(activeTab)

  const showSave = Boolean(activeTab)
  const saveDisabled = Boolean(activeTab?.etag !== undefined && !activeTab.isDirty)
  const hasSelection = Boolean(selection)
  const selectionStatements = hasSelection && isSqlTab ? splitSqlStatements(selection ?? '') : []

  async function handleSave() {
    if (!activeTab) return
    const result = await saveEditorTab(activeTab)
    if (result?.kind === 'save-as') setSaveAsTab(result.tab)
  }

  function handleFormat() {
    if (!activeTab || !activeGroupId) return
    const view = viewRegistry.get(`${activeGroupId}:${activeTab.id}`)
    if (!view) return
    try {
      formatEditorSql(view, sqlFormatterForDriver(activeTab.driver))
    } catch {
      toast.error('Could not format SQL.', {
        description: 'The query may contain unsupported or incomplete syntax.',
      })
    }
  }

  function handleSaveAsSuccess(tab: EditorTab, file: WorkspaceFile, etag: string) {
    const newTab: EditorTab = {
      id: `file:${file.id}`,
      workspaceId: workspace.id,
      title: file.name,
      kind: 'file',
      subtitle: file.name,
      fileId: file.id,
      content: tab.content,
      etag,
      isDirty: false,
    }
    openTab(newTab)
    closeTab(tab.id)
    setSaveAsTab(null)
  }

  const environments = useQuery(
    orgEnvironmentsQueryOptions(orgSlug, workspace.id, {
      page_size: 100,
      sort: 'name',
      order: 'asc',
    }),
  )
  const connections = useQuery(allOrgWorkspaceConnectionsQueryOptions(orgSlug, workspace.id))

  const envItems = environments.data?.items ?? []
  const connItems = connections.data?.items ?? []
  const activeConnection = connItems.find((c) => c.id === activeTab?.connectionId)
  const hasConnections = connItems.length > 0
  const activeConnectionSessionId = useIde((s) =>
    activeConnection ? s.sessions[activeConnection.id] : undefined,
  )
  const transactionMode = useTransactionMode(
    orgSlug,
    workspace.id,
    activeConnection?.id,
    activeConnectionSessionId,
  )
  useTransactionHydration(orgSlug, workspace.id, activeConnection?.id, activeConnectionSessionId)
  useEffect(() => {
    if (activeTab && activeConnection && activeTab.driver !== activeConnection.driver) {
      // Console tabs created before driver-aware completion retained only the
      // connection id. Repair those persisted tabs once connection metadata is
      // available so CodeMirror can register the correct dialect completer.
      setTabConnection(activeTab.id, activeConnection.id, activeConnection.driver)
    }
  }, [activeConnection, activeTab, setTabConnection])
  const queryAction = useToolbarQueryAction({
    orgSlug,
    workspace,
    activeTab,
    activeGroupId,
    activeConnection,
    hasConnections,
  })

  function selectConnection(conn: Connection) {
    if (activeTabId) setTabConnection(activeTabId, conn.id, conn.driver)
  }

  function toggleMaximize() {
    setMaximizedPane(maximizedPane === 'editor' ? null : 'editor')
  }

  function handleCancel() {
    queryAction.cancel()
  }
  const downloadNow = useDownloadNow(orgSlug, workspace.id)

  function handleExportClick() {
    const sql = queryAction.resolveSql()
    if (!sql) return
    setConfirmExportSql(sql)
  }

  function handleSaveFavoriteClick() {
    const sql = queryAction.resolveSql()
    if (!sql) return
    setSaveFavoriteSql(sql)
    setSaveFavoriteOpen(true)
  }

  function handleRunAll() {
    const text = queryAction.resolveDocumentText()
    const sqls = splitSqlStatements(text)
    if (sqls.length === 0) return
    void queryAction.runAll(sqls)
  }

  function handleRunSelectionAll() {
    if (selectionStatements.length === 0) return
    void queryAction.runAll(selectionStatements)
  }

  const runDisabled = !activeTab || !activeConnection || queryAction.isRunning
  return (
    <>
      <div className="flex h-10 shrink-0 items-center gap-2 border-b border-border bg-background px-2.5">
        {/* Run button — combined with quick-export options via the split arrow, when the active tab is a runnable query */}
        <div className="flex items-stretch">
          <Button
            type="button"
            className={cn('px-2.5', isSqlTab && 'rounded-r-none')}
            disabled={runDisabled}
            onClick={() => void queryAction.run()}
          >
            <Icon
              name={queryAction.isRunning ? 'loading-03' : 'play'}
              size={13}
              data-icon="inline-start"
              className={queryAction.isRunning ? 'animate-spin' : undefined}
            />
            {queryAction.isRunning ? 'Running…' : 'Run'}
          </Button>

          {isSqlTab &&
            (downloadNow.isDownloading ? (
              <Tip
                label={`Exporting… ${formatBytes(downloadNow.bytesDownloaded)} — click to cancel`}
              >
                <Button
                  type="button"
                  className="rounded-l-none border-l border-l-primary-foreground/20 px-2"
                  aria-label="Cancel export"
                  onClick={downloadNow.cancel}
                >
                  <Icon name="loading-03" size={13} className="animate-spin" />
                </Button>
              </Tip>
            ) : (
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <Button
                      type="button"
                      className="rounded-l-none border-l border-l-primary-foreground/20 px-1.5"
                      aria-label="More run options"
                      disabled={runDisabled}
                    />
                  }
                >
                  <Icon name="chevron-down" size={12} />
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start">
                  <DropdownMenuItem onClick={handleRunAll} disabled={runDisabled}>
                    <Icon name="play" size={13} data-icon="inline-start" />
                    Run All
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={queryAction.handleExplainClick}
                    disabled={runDisabled || !queryAction.canExplain}
                  >
                    <Icon name="search-01" size={13} data-icon="inline-start" />
                    Explain
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={queryAction.handleExplainAnalyzeClick}
                    disabled={runDisabled || !queryAction.canExplainAnalyze}
                  >
                    <Icon name="search-01" size={13} data-icon="inline-start" />
                    Explain Analyze
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={handleExportClick}>
                    <Icon name="download-01" size={13} data-icon="inline-start" />
                    Export
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={() => setExportToWorkspaceOpen(true)}>
                    <Icon name="folder" size={13} data-icon="inline-start" />
                    Export to workspace
                  </DropdownMenuItem>
                  <DropdownMenuItem onClick={handleSaveFavoriteClick}>
                    <Icon name="star" size={13} data-icon="inline-start" />
                    Save as favorite
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ))}
        </div>

        {selectionStatements.length > 1 && (
          <Tip label="Run every statement in the selection">
            <Button
              type="button"
              variant="outline"
              disabled={runDisabled}
              aria-label="Run all selected statements"
              onClick={handleRunSelectionAll}
            >
              <Icon name="play" size={13} data-icon="inline-start" />
              Run All
            </Button>
          </Tip>
        )}

        {/* Cancel button — appears only while a query is in flight */}
        {queryAction.isRunning && (
          <Button type="button" variant="outline" onClick={handleCancel}>
            <Icon name="cancel-01" size={13} data-icon="inline-start" />
            Cancel
          </Button>
        )}

        {/* Save button */}
        {showSave && (
          <Tip label="Save">
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Save file"
              disabled={saveDisabled}
              onClick={handleSave}
            >
              <Icon name="floppy-disk" size={13} />
            </Button>
          </Tip>
        )}

        {isSqlTab && (
          <Tip label="Format SQL">
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Format SQL"
              onClick={handleFormat}
              disabled={!activeGroupId}
            >
              <Icon name="subject" size={13} />
            </Button>
          </Tip>
        )}

        <div className="flex-1" />

        {activeConnection && activeConnectionSessionId && (
          <TransactionControls
            state={transactionMode.state}
            driver={activeConnection.driver}
            switchToManual={transactionMode.switchToManual}
            switchToAuto={transactionMode.switchToAuto}
            commit={transactionMode.commit}
            rollback={transactionMode.rollback}
            onSwitchToAutoBlocked={() => setSwitchToAutoGuardOpen(true)}
          />
        )}

        <ConnectionSelector
          activeConnection={activeConnection}
          activeConnectionId={activeTab?.connectionId}
          connections={connItems}
          environments={envItems}
          isLoading={connections.isLoading}
          tabAvailable={Boolean(activeTab)}
          onSelect={selectConnection}
        />

        {/* Maximize toggle */}
        <Tip label={maximizedPane === 'editor' ? 'Restore layout' : 'Maximize editor'}>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label="Toggle editor maximize"
            onClick={toggleMaximize}
          >
            <Icon name={maximizedPane === 'editor' ? 'minimize' : 'maximize'} size={14} />
          </Button>
        </Tip>
      </div>

      {saveAsTab && (
        <SaveAsDialog
          open={true}
          onOpenChange={(open) => {
            if (!open) setSaveAsTab(null)
          }}
          tab={saveAsTab}
          orgSlug={orgSlug}
          workspaceId={workspace.id}
          onSuccess={(file, etag) => handleSaveAsSuccess(saveAsTab, file, etag)}
        />
      )}

      {saveFavoriteOpen && (
        <SaveFavoriteDialog
          open={true}
          onOpenChange={setSaveFavoriteOpen}
          orgSlug={orgSlug}
          workspaceId={workspace.id}
          sqlText={saveFavoriteSql}
          connectionId={activeConnection?.id ?? null}
        />
      )}

      {activeConnection && confirmExportSql !== null && (
        <ExportConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) setConfirmExportSql(null)
          }}
          sql={confirmExportSql}
          onConfirm={() => {
            void downloadNow.download(activeConnection.id, confirmExportSql)
            setConfirmExportSql(null)
          }}
        />
      )}

      {activeConnection && queryAction.explainAnalyzeConfirmSql !== null && (
        <ExplainAnalyzeConfirmDialog
          open
          onOpenChange={(open) => {
            if (!open) queryAction.closeExplainAnalyzeConfirm()
          }}
          sql={queryAction.explainAnalyzeConfirmSql}
          onConfirm={queryAction.confirmExplainAnalyze}
        />
      )}

      {activeConnection && exportToWorkspaceOpen && (
        <ExportToFilesDialog
          open
          onOpenChange={setExportToWorkspaceOpen}
          orgSlug={orgSlug}
          workspaceId={workspace.id}
          connectionId={activeConnection.id}
          getSql={queryAction.resolveSql}
        />
      )}

      <TransactionGuardDialog
        open={switchToAutoGuardOpen}
        reason="switch-to-auto"
        pendingStatements={transactionMode.state.pendingStatements}
        onOpenChange={setSwitchToAutoGuardOpen}
        onCommit={() => {
          void (async () => {
            await transactionMode.commit()
            await transactionMode.switchToAuto()
            setSwitchToAutoGuardOpen(false)
          })()
        }}
        onRollback={() => {
          void (async () => {
            await transactionMode.rollback()
            await transactionMode.switchToAuto()
            setSwitchToAutoGuardOpen(false)
          })()
        }}
      />

      {activeTab && pendingConfirmation && (
        <UnsafeQueryDialog
          open
          onOpenChange={(open) => {
            if (!open) abandonPendingRunConfirmation(activeTab.id)
          }}
          sql={pendingConfirmation.sql}
          onConfirm={() => {
            const index = pendingConfirmation.statementIndex
            clearPendingConfirmation(activeTab.id)
            void queryAction.confirmAt(index)
          }}
        />
      )}
    </>
  )
}

// ─── SQL statement extraction ──────────────────────────────────────────────────

export { sqlStatementAtCursor, countSqlStatements } from './sqlStatements'
