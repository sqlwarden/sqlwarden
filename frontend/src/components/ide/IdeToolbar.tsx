import { useState, useCallback, useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Icon } from '#/lib/icons'
import { Button } from '#/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '#/components/ui/popover'
import {
  orgEnvironmentsQueryOptions,
  orgWorkspaceConnectionsQueryOptions,
} from '#/lib/api/query'
import { api } from '#/lib/api/client'
import { updatePrivateWorkspaceFileContent } from '#/lib/api/files'
import type { Connection, ResultSet, Workspace, WorkspaceFile } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { useIde, activeTabId as selectActiveTabId, type EditorTab } from './useIdeStore'
import { DriverBadge } from './DriverBadge'
import { SaveAsDialog } from './SaveAsDialog'
import { useYDocRegistry } from './useYDocRegistry'
import { useEditorViewRegistry } from './useEditorViewRegistry'

type IdeToolbarProps = {
  orgSlug: string
  workspace: Workspace
}

export function IdeToolbar({ orgSlug, workspace }: IdeToolbarProps) {
  const [popoverOpen, setPopoverOpen] = useState(false)
  const [connSearch, setConnSearch] = useState('')
  const [saveAsTab, setSaveAsTab] = useState<EditorTab | null>(null)

  const activeTabId = useIde((s) => selectActiveTabId(s, workspace.id))
  const activeGroupId = useIde((s) => s.activeGroupId[workspace.id])
  const tabs = useIde((s) => s.tabs)
  const openTab = useIde((s) => s.openTab)
  const closeTab = useIde((s) => s.closeTab)
  const setTabConnection = useIde((s) => s.setTabConnection)
  const updateTabEtag = useIde((s) => s.updateTabEtag)
  const maximizedPane = useIde((s) => s.maximizedPane)
  const setMaximizedPane = useIde((s) => s.setMaximizedPane)
  const sessions = useIde((s) => s.sessions)
  const setSession = useIde((s) => s.setSession)
  const setConnectionStatus = useIde((s) => s.setConnectionStatus)
  const setQueryResult = useIde((s) => s.setQueryResult)
  const setTabRunning = useIde((s) => s.setTabRunning)
  const setTabController = useIde((s) => s.setTabController)
  const runningTabs = useIde((s) => s.runningTabs)
  const abortControllers = useIde((s) => s.abortControllers)

  const registry = useYDocRegistry()
  const viewRegistry = useEditorViewRegistry()
  const activeTab = tabs.find((t) => t.id === activeTabId)
  const isRunning = !!(activeTabId && runningTabs[activeTabId])

  const showSave = activeTab?.kind !== 'file' || activeTab?.isDirty

  async function handleSave() {
    if (!activeTab) return
    const doc = registry.get(activeTab.id)
    const content = doc ? doc.getText('content').toString() : activeTab.content

    if (activeTab.kind === 'file' && activeTab.etag && activeTab.fileId) {
      try {
        const result = await updatePrivateWorkspaceFileContent(
          orgSlug,
          workspace.id,
          activeTab.fileId,
          content,
          activeTab.etag,
        )
        updateTabEtag(activeTab.id, result.etag)
      } catch (err) {
        const status = (err as { status?: number }).status
        if (status === 412 || status === 409) {
          toast.error('File changed externally. Reload before saving.')
        } else {
          toast.error('Failed to save file.')
        }
      }
    } else {
      setSaveAsTab({ ...activeTab, content })
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
    orgEnvironmentsQueryOptions(orgSlug, workspace.id, { page_size: 100, sort: 'name', order: 'asc' }),
  )
  const connections = useQuery(
    orgWorkspaceConnectionsQueryOptions(orgSlug, workspace.id, { page_size: 100, sort: 'name', order: 'asc' }),
  )

  const envItems = environments.data?.items ?? []
  const connItems = connections.data?.items ?? []
  const activeConnection = connItems.find((c) => c.id === activeTab?.connectionId)
  const hasConnections = connItems.length > 0

  function selectConnection(conn: Connection) {
    if (activeTabId) setTabConnection(activeTabId, conn.id, conn.driver)
    setPopoverOpen(false)
    setConnSearch('')
  }

  function toggleMaximize() {
    setMaximizedPane(maximizedPane === 'editor' ? null : 'editor')
  }

  function handleCancel() {
    if (!activeTabId) return
    abortControllers[activeTabId]?.abort()
  }

  const handleRun = useCallback(async () => {
    if (!activeTab || !activeConnection || isRunning) return

    const view = viewRegistry.get(activeGroupId ? `${activeGroupId}:${activeTab.id}` : activeTab.id)
    let sql: string
    if (view) {
      const sel = view.state.selection.main
      if (sel.from !== sel.to) {
        // Explicit selection — run exactly that text.
        sql = view.state.sliceDoc(sel.from, sel.to).trim()
      } else {
        // No selection — run the statement the cursor is inside.
        sql = sqlStatementAtCursor(view.state.doc.toString(), sel.head)
      }
    } else {
      const doc = registry.get(activeTab.id)
      sql = (doc ? doc.getText('content').toString() : activeTab.content).trim()
    }
    if (!sql) return

    // Ensure results pane is visible.
    if (maximizedPane === 'editor') setMaximizedPane(null)

    // Abort any stale controller for this tab (safety net).
    abortControllers[activeTab.id]?.abort()

    const controller = new AbortController()
    setTabController(activeTab.id, controller)
    setTabRunning(activeTab.id, true)
    setQueryResult(activeTab.id, { status: 'running' })

    try {
      // Auto-connect if no live session.
      let sessionId = sessions[activeConnection.id]
      if (!sessionId) {
        setConnectionStatus(activeConnection.id, 'connecting')
        try {
          const connectData = await api.post<{ session_id: string; reused: boolean }>(
            `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/connections/${activeConnection.id}/connect`,
            undefined,
            { signal: controller.signal },
          )
          sessionId = connectData.session_id
          setSession(activeConnection.id, sessionId)
        } finally {
          // Clears 'connecting'; on success the new session drives 'connected',
          // on failure it returns to idle (the query error surfaces in results).
          setConnectionStatus(activeConnection.id, null)
        }
      }

      const result = await api.post<ResultSet>(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/connections/${activeConnection.id}/query`,
        { sql },
        { headers: { 'X-Warden-Session': sessionId }, signal: controller.signal },
      )

      setQueryResult(activeTab.id, {
        status: 'ok',
        data: result,
        durationMs: result.duration_ms,
        sql,
      })
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        setQueryResult(activeTab.id, { status: 'cancelled', sql })
      } else {
        const message = err instanceof Error ? err.message : 'Query failed'
        setQueryResult(activeTab.id, { status: 'error', message, sql })
      }
    } finally {
      setTabController(activeTab.id, null)
      setTabRunning(activeTab.id, false)
    }
  }, [activeTab, activeGroupId, activeConnection, isRunning, maximizedPane, sessions, orgSlug, workspace.id,
      registry, viewRegistry, abortControllers, setMaximizedPane, setQueryResult, setSession, setConnectionStatus,
      setTabController, setTabRunning])

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
    <div className="flex h-10 shrink-0 items-center gap-2 border-b border-border px-2">
      {/* Run button — always shown; disabled while running */}
      <Button
        type="button"
        size="sm"
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
      </Button>

      {/* Cancel button — appears only while a query is in flight */}
      {isRunning && (
        <Button
          type="button"
          size="sm"
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
          size="sm"
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
        <PopoverTrigger
          render={
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={selectorDisabled}
              className="h-7 min-w-0 max-w-56 gap-1.5 rounded-md border border-border/60 px-2 text-xs font-normal hover:border-border"
            />
          }
        >
          {activeConnection ? (
            <>
              <DriverBadge driver={activeConnection.driver} size="sm" className="shrink-0" />
              <span className="min-w-0 flex-1 truncate">{activeConnection.name}</span>
              {sessions[activeConnection.id] && (
                <span className="size-1.5 shrink-0 rounded-full bg-green-500" />
              )}
            </>
          ) : (
            <>
              <Icon name="database" size={12} className="shrink-0 text-muted-foreground" />
              <span className="text-muted-foreground">{selectorLabel}</span>
            </>
          )}
          <Icon name="arrow-down-01" size={10} className="ml-0.5 shrink-0 text-muted-foreground" />
        </PopoverTrigger>

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
    </>
  )
}

// ─── SQL statement extraction ──────────────────────────────────────────────────

/**
 * Returns the SQL statement that contains `cursor` (a character offset).
 * Scans for semicolons that are not inside string literals or comments.
 * Falls back to the full trimmed text when no semicolons are present.
 *
 * Statement spans are [inclusive_start, exclusive_end]. When the cursor
 * sits in whitespace between statements, the preceding statement wins.
 */
export function sqlStatementAtCursor(text: string, cursor: number): string {
  type LexState = 'normal' | 'sq' | 'dq' | 'lc' | 'bc'
  let state: LexState = 'normal'
  const semis: number[] = []

  for (let i = 0; i < text.length; i++) {
    const c = text[i]
    const n = text[i + 1] ?? ''
    switch (state) {
      case 'normal':
        if (c === "'") { state = 'sq' }
        else if (c === '"') { state = 'dq' }
        else if (c === '-' && n === '-') { state = 'lc'; i++ }
        else if (c === '/' && n === '*') { state = 'bc'; i++ }
        else if (c === ';') { semis.push(i) }
        break
      case 'sq':
        if (c === "'" && n === "'") { i++ } // escaped ''
        else if (c === "'") { state = 'normal' }
        break
      case 'dq':
        if (c === '"' && n === '"') { i++ } // escaped ""
        else if (c === '"') { state = 'normal' }
        break
      case 'lc':
        if (c === '\n') { state = 'normal' }
        break
      case 'bc':
        if (c === '*' && n === '/') { state = 'normal'; i++ }
        break
    }
  }

  if (semis.length === 0) return text.trim()

  // Build [start, exclusive_end] spans from semicolon positions.
  // Each next span starts at the first non-whitespace char after the semicolon
  // so that the gap between statements belongs to no span — the cursor-in-gap
  // case then correctly falls back to the preceding statement.
  const spans: Array<[number, number]> = []
  let start = 0
  for (const semi of semis) {
    spans.push([start, semi + 1]) // include the semicolon in the span
    let nextStart = semi + 1
    while (nextStart < text.length && /\s/.test(text[nextStart])) nextStart++
    start = nextStart
  }
  // Trailing content after the last semicolon (statement without terminator).
  if (text.slice(start).trim()) {
    spans.push([start, text.length])
  }

  // Find the span that contains the cursor.
  for (const [s, e] of spans) {
    if (cursor >= s && cursor < e) return text.slice(s, e).trim()
  }

  // Cursor is in trailing whitespace — return the last span that started at or
  // before the cursor (i.e. the statement immediately preceding it).
  let best = spans[0]
  for (const span of spans) {
    if (span[0] <= cursor) best = span
  }
  return text.slice(best[0], best[1]).trim()
}
