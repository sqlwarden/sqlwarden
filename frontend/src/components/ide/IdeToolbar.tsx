import { useState, useCallback, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Icon } from '#/lib/icons'
import { Button } from '#/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '#/components/ui/dropdown-menu'
import { Popover, PopoverContent, PopoverTrigger } from '#/components/ui/popover'
import {
  orgEnvironmentsQueryOptions,
  allOrgWorkspaceConnectionsQueryOptions,
} from '#/lib/api/query'
import type { Connection, Workspace, WorkspaceFile } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { useIde, activeTabId as selectActiveTabId, type EditorTab } from './useIdeStore'
import { sqlStatementAtCursor } from './sqlStatements'
import { DriverBadge } from './DriverBadge'
import { SaveAsDialog } from './SaveAsDialog'
import { ExportConfirmDialog } from './exports/ExportConfirmDialog'
import { ExportToFilesDialog } from './exports/ExportToFilesDialog'
import { formatBytes } from './exports/formatBytes'
import { useDownloadNow } from './exports/useDownloadNow'
import { Tip } from './schema-diagram/Tip'
import { useQueryExecution } from './useQueryExecution'
import { useYDocRegistry } from './useYDocRegistry'
import { useEditorViewRegistry } from './useEditorViewRegistry'
import { useSaveEditorTab } from './useSaveEditorTab'

type IdeToolbarProps = {
  orgSlug: string
  workspace: Workspace
}

export const RUN_SHORTCUT =
  typeof navigator !== 'undefined' && /mac/i.test(navigator.platform) ? '⌘↵' : 'Ctrl ↵'

export function IdeToolbar({ orgSlug, workspace }: IdeToolbarProps) {
  const [popoverOpen, setPopoverOpen] = useState(false)
  const [connSearch, setConnSearch] = useState('')
  const [saveAsTab, setSaveAsTab] = useState<EditorTab | null>(null)
  const [confirmExportSql, setConfirmExportSql] = useState<string | null>(null)
  const [exportToWorkspaceOpen, setExportToWorkspaceOpen] = useState(false)

  const activeTabId = useIde((s) => selectActiveTabId(s, workspace.id))
  const activeGroupId = useIde((s) => s.activeGroupId[workspace.id])
  const tabs = useIde((s) => s.tabs)
  const openTab = useIde((s) => s.openTab)
  const closeTab = useIde((s) => s.closeTab)
  const setTabConnection = useIde((s) => s.setTabConnection)
  const maximizedPane = useIde((s) => s.maximizedPane)
  const setMaximizedPane = useIde((s) => s.setMaximizedPane)
  const sessions = useIde((s) => s.sessions)

  const registry = useYDocRegistry()
  const viewRegistry = useEditorViewRegistry()
  const saveEditorTab = useSaveEditorTab(orgSlug, workspace.id)
  const activeTab = tabs.find((t) => t.id === activeTabId)

  // Export only makes sense for a runnable SQL query — not for a database
  // object/diagram tab or a non-SQL file (e.g. a previously exported CSV).
  const isSqlTab = !!activeTab && (
    activeTab.kind === 'scratch' ||
    activeTab.kind === 'connection' ||
    (activeTab.kind === 'file' && activeTab.title.toLowerCase().endsWith('.sql'))
  )

  const showSave = activeTab?.kind !== 'file' || activeTab?.isDirty

  async function handleSave() {
    if (!activeTab) return
    const result = await saveEditorTab(activeTab)
    if (result?.kind === 'save-as') setSaveAsTab(result.tab)
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
    orgEnvironmentsQueryOptions(orgSlug, workspace.id, { page_size: 100, sort: 'name', order: 'asc' }),
  )
  const connections = useQuery(
    allOrgWorkspaceConnectionsQueryOptions(orgSlug, workspace.id),
  )

  const envItems = environments.data?.items ?? []
  const connItems = connections.data?.items ?? []
  const activeConnection = connItems.find((c) => c.id === activeTab?.connectionId)
  const hasConnections = connItems.length > 0
  const {
    cancel: cancelQuery,
    isRunning,
    run: runQuery,
  } = useQueryExecution(
    orgSlug,
    workspace.id,
    activeTab?.id,
    activeConnection?.id,
  )

  function selectConnection(conn: Connection) {
    if (activeTabId) setTabConnection(activeTabId, conn.id, conn.driver)
    setPopoverOpen(false)
    setConnSearch('')
  }

  function toggleMaximize() {
    setMaximizedPane(maximizedPane === 'editor' ? null : 'editor')
  }

  function handleCancel() {
    cancelQuery()
  }
  const downloadNow = useDownloadNow(orgSlug, workspace.id)

  const resolveSql = useCallback((): string => {
    if (!activeTab) return ''
    const view = viewRegistry.get(activeGroupId ? `${activeGroupId}:${activeTab.id}` : activeTab.id)
    if (view) {
      const sel = view.state.selection.main
      if (sel.from !== sel.to) {
        // Explicit selection — run exactly that text.
        return view.state.sliceDoc(sel.from, sel.to).trim()
      }
      // No selection — run the statement the cursor is inside.
      return sqlStatementAtCursor(view.state.doc.toString(), sel.head)
    }
    const doc = registry.get(activeTab.id)
    return (doc ? doc.getText('content').toString() : activeTab.content).trim()
  }, [activeTab, activeGroupId, viewRegistry, registry])

  function handleExportClick() {
    const sql = resolveSql()
    if (!sql) return
    setConfirmExportSql(sql)
  }

  const handleRun = useCallback(async () => {
    if (!activeTab || isRunning) return
    // The Run button is disabled without a connection, so this guard only trips
    // on the keyboard shortcut — tell the user why nothing happened.
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

    // Ensure results pane is visible.
    if (maximizedPane === 'editor') setMaximizedPane(null)

    await runQuery(sql)
  }, [activeTab, activeConnection, hasConnections, isRunning, maximizedPane,
      resolveSql, setMaximizedPane, runQuery])

  // Global ⌘Enter / Ctrl+Enter shortcut.
  // capture:true fires before CodeMirror's contentDOM listener; stopPropagation
  // prevents the event from reaching CodeMirror at all, so it cannot insert a newline.
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
        e.preventDefault()
        e.stopPropagation()
        void handleRun()
      }
    }
    window.addEventListener('keydown', onKeyDown, { capture: true })
    return () => window.removeEventListener('keydown', onKeyDown, { capture: true })
  }, [handleRun])

  const runDisabled = !activeTab || !activeConnection || isRunning
  const selectorDisabled = !activeTab || !hasConnections || connections.isLoading
  const selectorLabel = (() => {
    if (connections.isLoading) return 'Loading connections…'
    if (!hasConnections) return 'No connections'
    if (activeConnection) return null
    return 'Select connection…'
  })()

  return (
    <>
    <div className="flex h-10 shrink-0 items-center gap-2 border-b border-border bg-background px-2.5">
      {/* Run button — combined with quick-export options via the split arrow, when the active tab is a runnable query */}
      <div className="flex items-stretch">
        <Button
          type="button"
          className={cn('px-2.5', isSqlTab && 'rounded-r-none')}
          disabled={runDisabled}
          onClick={() => void handleRun()}
        >
          <Icon
            name={isRunning ? 'loading-03' : 'play'}
            size={13}
            data-icon="inline-start"
            className={isRunning ? 'animate-spin' : undefined}
          />
          {isRunning ? 'Running…' : 'Run'}
          {!isRunning && (
            <kbd className="ml-0.5 hidden rounded bg-primary-foreground/20 px-1 font-sans text-[9px] font-medium leading-4 tracking-wide sm:inline">
              {RUN_SHORTCUT}
            </kbd>
          )}
        </Button>

        {isSqlTab && (
          downloadNow.isDownloading ? (
            <Tip label={`Exporting… ${formatBytes(downloadNow.bytesDownloaded)} — click to cancel`}>
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
                <DropdownMenuItem onClick={handleExportClick}>
                  <Icon name="download-01" size={13} data-icon="inline-start" />
                  Export
                </DropdownMenuItem>
                <DropdownMenuItem onClick={() => setExportToWorkspaceOpen(true)}>
                  <Icon name="folder" size={13} data-icon="inline-start" />
                  Export to workspace
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          )
        )}
      </div>

      {/* Cancel button — appears only while a query is in flight */}
      {isRunning && (
        <Button
          type="button"
          variant="outline"
          onClick={handleCancel}
        >
          <Icon name="cancel-01" size={13} data-icon="inline-start" />
          Cancel
        </Button>
      )}

      {/* Save button */}
      {showSave && (
        <Button
          type="button"
          variant="outline"
          aria-label="Save file"
          onClick={handleSave}
        >
          <Icon name="floppy-disk" size={13} data-icon="inline-start" />
          Save
        </Button>
      )}

      <div className="flex-1" />

      {/* Connection selector — right */}
      <Popover
        open={popoverOpen}
        onOpenChange={(open) => { setPopoverOpen(open); if (!open) setConnSearch('') }}
      >
        <Tip
          label={
            activeConnection
              ? sessions[activeConnection.id]
                ? `Connected to ${activeConnection.name}`
                : 'Not connected — Run connects automatically'
              : 'Choose a connection to run queries'
          }
        >
          <PopoverTrigger
            render={
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={selectorDisabled}
                className="h-7 min-w-0 max-w-60 gap-1.5 px-2 text-xs font-normal"
              />
            }
          >
            {activeConnection ? (
              <>
                <DriverBadge driver={activeConnection.driver} size="sm" className="shrink-0" />
                <span className="min-w-0 flex-1 truncate">{activeConnection.name}</span>
                <span
                  className={cn(
                    'size-1.5 shrink-0 rounded-full',
                    sessions[activeConnection.id] ? 'bg-green-500' : 'border border-muted-foreground/60',
                  )}
                />
              </>
            ) : (
              <>
                <Icon name="database" size={12} className="shrink-0 text-muted-foreground" />
                <span className="text-muted-foreground">{selectorLabel}</span>
              </>
            )}
            <Icon name="arrow-down-01" size={10} className="ml-0.5 shrink-0 text-muted-foreground" />
          </PopoverTrigger>
        </Tip>

        <PopoverContent align="end" className="w-72 p-0 overflow-hidden">
          {connections.isLoading ? (
            <div className="px-3 py-4 text-center text-xs text-muted-foreground">Loading connections…</div>
          ) : !hasConnections ? (
            <div className="px-3 py-4 text-center text-xs text-muted-foreground">
              <p className="font-medium text-foreground">No connections</p>
              <p className="mt-0.5">Add a connection to this workspace first.</p>
            </div>
          ) : (
            <>
              {/* Search */}
              <div className="flex items-center gap-2 border-b border-border px-3 py-2">
                <Icon name="search-01" size={12} className="shrink-0 text-muted-foreground" />
                <input
                  type="text"
                  placeholder="Search connections…"
                  value={connSearch}
                  onChange={(e) => setConnSearch(e.target.value)}
                  className="min-w-0 flex-1 bg-transparent text-xs outline-none placeholder:text-muted-foreground"
                  autoFocus
                />
              </div>

              {/* Connection list */}
              <div className="max-h-72 overflow-y-auto py-1">
                {envItems.map((env) => {
                  const q = connSearch.toLowerCase()
                  const envConns = connItems.filter(
                    (c) => c.environment_id === env.id &&
                      (!q || c.name.toLowerCase().includes(q) || env.name.toLowerCase().includes(q))
                  )
                  if (!envConns.length) return null
                  return (
                    <div key={env.id} className="mb-1 last:mb-0">
                      <div className="flex items-center gap-1.5 px-3 pb-1 pt-2">
                        <Icon name="server-stack-01" size={11} className="shrink-0 text-muted-foreground/70" />
                        <span className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground/70">
                          {env.name}
                        </span>
                      </div>
                      {envConns.map((conn) => {
                        const isActive = activeTab?.connectionId === conn.id
                        const isConnected = !!sessions[conn.id]
                        return (
                          <button
                            key={conn.id}
                            type="button"
                            onClick={() => selectConnection(conn)}
                            className={cn(
                              'flex h-8 w-full items-center gap-2.5 px-3 text-xs transition-colors',
                              'hover:bg-accent hover:text-accent-foreground',
                              isActive && 'bg-accent/60 text-accent-foreground',
                            )}
                          >
                            <DriverBadge driver={conn.driver} size="sm" className="shrink-0" />
                            <span className="min-w-0 flex-1 truncate text-left">{conn.name}</span>
                            {isConnected && (
                              <span className="size-1.5 shrink-0 rounded-full bg-green-500" />
                            )}
                            {isActive && (
                              <Icon name="checkmark-circle-02" size={13} className="shrink-0 text-primary" />
                            )}
                          </button>
                        )
                      })}
                    </div>
                  )
                })}
                {connSearch && !envItems.some((env) =>
                  connItems.some((c) => c.environment_id === env.id && c.name.toLowerCase().includes(connSearch.toLowerCase()))
                ) && (
                  <div className="px-3 py-3 text-center text-xs text-muted-foreground">
                    No connections match "{connSearch}"
                  </div>
                )}
              </div>
            </>
          )}
        </PopoverContent>
      </Popover>

      {/* Maximize toggle */}
      <Tip label={maximizedPane === 'editor' ? 'Restore layout' : 'Maximize editor'}>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="Toggle editor maximize"
          onClick={toggleMaximize}
        >
          <Icon
            name={maximizedPane === 'editor' ? 'minimize' : 'maximize'}
            size={14}
          />
        </Button>
      </Tip>
    </div>

    {saveAsTab && (
      <SaveAsDialog
        open={true}
        onOpenChange={(open) => { if (!open) setSaveAsTab(null) }}
        tab={saveAsTab}
        orgSlug={orgSlug}
        workspaceId={workspace.id}
        onSuccess={(file, etag) => handleSaveAsSuccess(saveAsTab, file, etag)}
      />
    )}

    {activeConnection && confirmExportSql !== null && (
      <ExportConfirmDialog
        open
        onOpenChange={(open) => { if (!open) setConfirmExportSql(null) }}
        sql={confirmExportSql}
        onConfirm={() => {
          void downloadNow.download(activeConnection.id, confirmExportSql)
          setConfirmExportSql(null)
        }}
      />
    )}

    {activeConnection && exportToWorkspaceOpen && (
      <ExportToFilesDialog
        open
        onOpenChange={setExportToWorkspaceOpen}
        orgSlug={orgSlug}
        workspaceId={workspace.id}
        connectionId={activeConnection.id}
        getSql={resolveSql}
      />
    )}
    </>
  )
}

// ─── SQL statement extraction ──────────────────────────────────────────────────

export { sqlStatementAtCursor, countSqlStatements } from './sqlStatements'
