import { useState, useEffect, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  ArrowExpandIcon,
  ArrowShrinkIcon,
  DatabaseIcon,
  FloppyDiskIcon,
  Loading03Icon,
  PlayIcon,
  ServerStack01Icon,
} from '@hugeicons/core-free-icons'
import { Button } from '#/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '#/components/ui/popover'
import { Separator } from '#/components/ui/separator'
import {
  orgEnvironmentsQueryOptions,
  orgWorkspaceConnectionsQueryOptions,
} from '#/lib/api/query'
import { api } from '#/lib/api/client'
import { updatePrivateWorkspaceFileContent } from '#/lib/api/files'
import type { Connection, ResultSet, Workspace, WorkspaceFile } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { useIde, type EditorTab } from './useIdeStore'
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
  const [saveAsTab, setSaveAsTab] = useState<EditorTab | null>(null)
  const [isRunning, setIsRunning] = useState(false)

  const activeTabId = useIde((s) => s.activeTabId)
  const tabs = useIde((s) => s.tabs)
  const openTab = useIde((s) => s.openTab)
  const closeTab = useIde((s) => s.closeTab)
  const setTabConnection = useIde((s) => s.setTabConnection)
  const updateTabEtag = useIde((s) => s.updateTabEtag)
  const maximizedPane = useIde((s) => s.maximizedPane)
  const setMaximizedPane = useIde((s) => s.setMaximizedPane)
  const sessions = useIde((s) => s.sessions)
  const setSession = useIde((s) => s.setSession)
  const setQueryResult = useIde((s) => s.setQueryResult)

  const registry = useYDocRegistry()
  const viewRegistry = useEditorViewRegistry()
  const activeTab = tabs.find((t) => t.id === activeTabId)

  const showSave = activeTab?.kind === 'scratch' || activeTab?.isDirty

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
  const activeEnv = envItems.find((e) => e.id === activeConnection?.environment_id)
  const hasConnections = connItems.length > 0

  function selectConnection(conn: Connection) {
    if (activeTabId) setTabConnection(activeTabId, conn.id)
    setPopoverOpen(false)
  }

  function toggleMaximize() {
    setMaximizedPane(maximizedPane === 'editor' ? null : 'editor')
  }

  const handleRun = useCallback(async () => {
    if (!activeTab || !activeConnection || isRunning) return

    const view = viewRegistry.get(activeTab.id)
    let sql: string
    if (view) {
      const sel = view.state.selection.main
      sql = (sel.from !== sel.to
        ? view.state.sliceDoc(sel.from, sel.to)
        : view.state.doc.toString()
      ).trim()
    } else {
      const doc = registry.get(activeTab.id)
      sql = (doc ? doc.getText('content').toString() : activeTab.content).trim()
    }
    if (!sql) return

    // Ensure results pane is visible.
    if (maximizedPane === 'editor') setMaximizedPane(null)

    setIsRunning(true)
    setQueryResult(activeTab.id, { status: 'running' })

    const startMs = Date.now()

    try {
      // Auto-connect if no live session.
      let sessionId = sessions[activeConnection.id]
      if (!sessionId) {
        const connectData = await api.post<{ session_id: string; reused: boolean }>(
          `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/connections/${activeConnection.id}/connect`,
        )
        sessionId = connectData.session_id
        setSession(activeConnection.id, sessionId)
      }

      const result = await api.post<ResultSet>(
        `/api/v1/orgs/${orgSlug}/workspaces/${workspace.id}/connections/${activeConnection.id}/query`,
        { sql },
        { headers: { 'X-Warden-Session': sessionId } },
      )

      setQueryResult(activeTab.id, {
        status: 'ok',
        data: result,
        durationMs: Date.now() - startMs,
        sql,
      })
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Query failed'
      setQueryResult(activeTab.id, { status: 'error', message, sql })
    } finally {
      setIsRunning(false)
    }
  }, [activeTab, activeConnection, isRunning, maximizedPane, sessions, orgSlug, workspace.id,
      registry, viewRegistry, setMaximizedPane, setQueryResult, setSession])

  // Global ⌘Enter / Ctrl+Enter shortcut.
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
        e.preventDefault()
        void handleRun()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
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
      {/* Run button */}
      <Button
        type="button"
        size="sm"
        disabled={runDisabled}
        onClick={() => void handleRun()}
      >
        <HugeiconsIcon
          icon={isRunning ? Loading03Icon : PlayIcon}
          size={13}
          strokeWidth={2}
          data-icon="inline-start"
          className={isRunning ? 'animate-spin' : undefined}
        />
        {isRunning ? 'Running…' : 'Run'}
        {!isRunning && <kbd className="ml-1 hidden font-mono text-[10px] opacity-60 sm:inline">⌘↵</kbd>}
      </Button>

      {/* Save button */}
      {showSave && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          aria-label="Save file"
          onClick={handleSave}
        >
          <HugeiconsIcon icon={FloppyDiskIcon} size={13} strokeWidth={2} data-icon="inline-start" />
          Save
          <kbd className="ml-1 hidden font-mono text-[10px] opacity-60 sm:inline">⌘S</kbd>
        </Button>
      )}

      <div className="flex-1" />

      {/* Connection selector — right */}
      <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
        <PopoverTrigger
          render={
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={selectorDisabled}
              className="h-7 min-w-0 max-w-64 gap-2 text-xs"
            />
          }
        >
          {activeConnection ? (
            <DriverBadge driver={activeConnection.driver} size="sm" className="shrink-0" />
          ) : (
            <HugeiconsIcon icon={DatabaseIcon} size={13} strokeWidth={2} className="shrink-0" />
          )}
          {activeConnection ? (
            <>
              <span className="truncate font-mono">{activeConnection.name}</span>
              {activeEnv && (
                <span className="shrink-0 text-[10px] text-muted-foreground">· {activeEnv.name}</span>
              )}
            </>
          ) : (
            <span className="text-muted-foreground">{selectorLabel}</span>
          )}
        </PopoverTrigger>
        <PopoverContent align="end" className="w-72 p-1">
          {connections.isLoading ? (
            <div className="px-2 py-3 text-xs text-muted-foreground">Loading connections…</div>
          ) : !hasConnections ? (
            <div className="px-2 py-3 text-center text-xs text-muted-foreground">
              <p className="font-medium text-foreground">No connections</p>
              <p className="mt-0.5">Add a connection to this workspace first.</p>
            </div>
          ) : (
            envItems.map((env, idx) => {
              const envConns = connItems.filter((c) => c.environment_id === env.id)
              if (!envConns.length) return null
              return (
                <div key={env.id}>
                  {idx > 0 && <Separator className="my-1" />}
                  <div className="flex items-center gap-1.5 px-2 py-1.5">
                    <HugeiconsIcon icon={ServerStack01Icon} size={12} strokeWidth={2} className="text-muted-foreground" />
                    <span className="text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
                      {env.name}
                    </span>
                  </div>
                  {envConns.map((conn) => (
                    <button
                      key={conn.id}
                      type="button"
                      onClick={() => selectConnection(conn)}
                      className={cn(
                        'flex h-7 w-full items-center gap-2 px-2 text-xs',
                        'transition-colors hover:bg-accent hover:text-accent-foreground',
                        activeTab?.connectionId === conn.id && 'bg-accent text-accent-foreground',
                      )}
                    >
                      <DriverBadge driver={conn.driver} size="sm" className="shrink-0" />
                      <span className="min-w-0 flex-1 truncate font-mono">{conn.name}</span>
                    </button>
                  ))}
                </div>
              )
            })
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
        <HugeiconsIcon
          icon={maximizedPane === 'editor' ? ArrowShrinkIcon : ArrowExpandIcon}
          size={14}
          strokeWidth={2}
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
