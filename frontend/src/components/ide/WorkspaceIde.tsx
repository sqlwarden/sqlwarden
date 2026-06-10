import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import * as Y from 'yjs'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Icon } from '#/lib/icons'
import { updatePrivateWorkspaceFileContent } from '#/lib/api/files'
import type { PanelImperativeHandle } from 'react-resizable-panels'
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '#/components/ui/resizable'
import { orgWorkspacesQueryOptions } from '#/lib/api/query'
import type { Workspace } from '#/lib/api/types'
import { useSession } from '#/hooks/use-session'
import { cn } from '#/lib/utils'
import {
  createIdeStore,
  IdeStoreContext,
  useIde,
  DEFAULT_CONSOLE_CONTENT,
  type EditorTab,
} from './useIdeStore'
import { SaveAsDialog } from './SaveAsDialog'
import { DatabasePanel } from './DatabasePanel'
import { FilesPanel } from './FilesPanel'
import { IdeToolbar } from './IdeToolbar'
import { IdeTabBar } from './IdeTabBar'
import { SqlEditor } from './SqlEditor'
import { ResultsArea } from './ResultsArea'
import { useFileContent } from './useFileContent'
import { createYDocRegistry, YDocRegistryContext, useYDocRegistry } from './useYDocRegistry'
import { createEditorViewRegistry, EditorViewRegistryContext } from './useEditorViewRegistry'

// ─── Root ──────────────────────────────────────────────────────────────────────

type WorkspaceIdeProps = { orgSlug: string }

export function WorkspaceIde({ orgSlug }: WorkspaceIdeProps) {
  // Parent route guards that session is loaded before rendering this component,
  // so session.data is always available here (cache hit, no network request).
  // Fall back to 0 defensively so hooks are never called with undefined deps.
  const { data: session } = useSession()
  const accountId = session?.account?.id ?? 0

  const store = useMemo(() => createIdeStore(orgSlug, accountId), [orgSlug, accountId])
  const registry = useMemo(() => createYDocRegistry(accountId), [orgSlug, accountId])
  const viewRegistry = useMemo(() => createEditorViewRegistry(), [])

  // ── Cross-window etag / dirty-state sync ─────────────────────────────────
  // When any window saves a file (etag changes) or completes a server load
  // (initial etag set), broadcast the new etag to all other windows on the
  // same origin so their dirty indicator and save ETag stay in sync.
  // The receiving window calls updateTabEtag directly — no re-broadcast.
  useEffect(() => {
    const channel = new BroadcastChannel(`sqlwarden:store:${orgSlug}:${accountId}`)
    const prevEtags = new Map<string, string>()
    const prevTabIds = new Set<string>()
    let prevSessions: Record<number, string> = {}
    let applyingRemote = false
    let seeded = false

    function handleRemote(event: MessageEvent) {
      const msg = event.data as Record<string, unknown>
      if (!msg?.type) return
      applyingRemote = true
      if (msg.type === 'etag-update' && typeof msg.tabId === 'string' && typeof msg.etag === 'string') {
        store.getState().updateTabEtag(msg.tabId, msg.etag)
      } else if (msg.type === 'tab-opened' && msg.tab) {
        store.getState().ensureTab(msg.tab as EditorTab)
      } else if (msg.type === 'tab-closed' && typeof msg.tabId === 'string') {
        store.getState().closeTab(msg.tabId)
      } else if (msg.type === 'session-set' && typeof msg.connectionId === 'number' && typeof msg.sessionId === 'string') {
        store.getState().setSession(msg.connectionId, msg.sessionId)
      } else if (msg.type === 'session-cleared' && typeof msg.connectionId === 'number') {
        store.getState().clearSession(msg.connectionId)
      }
      applyingRemote = false
    }

    const unsub = store.subscribe((state) => {
      const currentTabIds = new Set(state.tabs.map((t) => t.id))

      // First callback is the post-hydration snapshot. Seed prev state without
      // broadcasting so we don't mistake restored tabs for newly opened ones.
      if (!seeded) {
        seeded = true
        currentTabIds.forEach((id) => prevTabIds.add(id))
        for (const tab of state.tabs) {
          if (tab.etag !== undefined) prevEtags.set(tab.id, tab.etag)
        }
        prevSessions = { ...state.sessions }
        return
      }

      if (!applyingRemote) {
        // Etag changes
        for (const tab of state.tabs) {
          const prev = prevEtags.get(tab.id)
          if (tab.etag !== undefined && tab.etag !== prev) {
            channel.postMessage({ type: 'etag-update', tabId: tab.id, etag: tab.etag })
          }
        }
        // Scratch tab opens — file tabs are per-window, consoles are shared
        for (const tab of state.tabs) {
          if (!prevTabIds.has(tab.id) && tab.kind === 'scratch') {
            channel.postMessage({ type: 'tab-opened', tab })
          }
        }
        // Scratch tab closes
        for (const id of prevTabIds) {
          if (!currentTabIds.has(id) && id.startsWith('scratch:')) {
            channel.postMessage({ type: 'tab-closed', tabId: id })
          }
        }
        // Session changes — drives the green connected dot across windows
        for (const [connIdStr, sessionId] of Object.entries(state.sessions)) {
          const connId = Number(connIdStr)
          if (prevSessions[connId] !== sessionId) {
            channel.postMessage({ type: 'session-set', connectionId: connId, sessionId })
          }
        }
        for (const connIdStr of Object.keys(prevSessions)) {
          const connId = Number(connIdStr)
          if (!(connId in state.sessions)) {
            channel.postMessage({ type: 'session-cleared', connectionId: connId })
          }
        }
      }

      // Always update prev state (even when applyingRemote) to stay current.
      prevTabIds.clear()
      currentTabIds.forEach((id) => prevTabIds.add(id))
      for (const tab of state.tabs) {
        if (tab.etag !== undefined) prevEtags.set(tab.id, tab.etag)
      }
      prevSessions = { ...state.sessions }
    })

    channel.addEventListener('message', handleRemote)
    return () => {
      unsub()
      channel.removeEventListener('message', handleRemote)
      channel.close()
    }
  }, [store, orgSlug, accountId])

  // Abort all in-flight queries when the IDE unmounts (e.g. navigation away).
  useEffect(() => {
    return () => {
      const { abortControllers } = store.getState()
      Object.values(abortControllers).forEach((c) => c.abort())
    }
  }, [store])

  const workspaces = useQuery(
    orgWorkspacesQueryOptions(orgSlug, { page_size: 100, sort: 'name', order: 'asc' }),
  )

  const items = workspaces.data?.items ?? []

  // Always render providers so useIde never runs without context, even when
  // the workspaces query is still loading or the component suspends mid-render.
  return (
    <IdeStoreContext.Provider value={store}>
      <YDocRegistryContext.Provider value={registry}>
        <EditorViewRegistryContext.Provider value={viewRegistry}>
          {workspaces.isLoading ? (
            <IdeFrame>Loading workspaces…</IdeFrame>
          ) : workspaces.isError ? (
            <IdeFrame>Unable to load workspaces.</IdeFrame>
          ) : items.length === 0 ? (
            <IdeFrame>No accessible workspaces.</IdeFrame>
          ) : (
            <WorkspaceIdeInner orgSlug={orgSlug} workspaces={items} />
          )}
        </EditorViewRegistryContext.Provider>
      </YDocRegistryContext.Provider>
    </IdeStoreContext.Provider>
  )
}

// ─── Inner ─────────────────────────────────────────────────────────────────────

function WorkspaceIdeInner({ orgSlug, workspaces }: { orgSlug: string; workspaces: Workspace[] }) {
  const activeWorkspaceId = useIde((s) => s.activeWorkspaceId)
  const setActiveWorkspace = useIde((s) => s.setActiveWorkspace)

  useEffect(() => {
    if (!activeWorkspaceId && workspaces.length > 0) {
      setActiveWorkspace(workspaces[0].id)
    }
  }, [activeWorkspaceId, workspaces, setActiveWorkspace])

  const activeWorkspace =
    workspaces.find((w) => w.id === activeWorkspaceId) ?? workspaces[0]

  return (
    <div className="-mx-4 -my-6 flex h-svh min-h-0 flex-col overflow-hidden bg-background md:-mx-6">
      {/* Top bar: explorer toggle + workspace tabs */}
      <div className="flex h-10 shrink-0 items-stretch border-b border-border">
        <ExplorerToggle />
        <div className="flex min-w-0 flex-1 items-end overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          {workspaces.map((ws) => (
            <WorkspaceTab
              key={ws.id}
              workspace={ws}
              active={ws.id === activeWorkspace?.id}
              onActivate={() => setActiveWorkspace(ws.id)}
            />
          ))}
        </div>
      </div>

      {activeWorkspace && (
        <WorkspaceIdeSurface orgSlug={orgSlug} workspace={activeWorkspace} />
      )}
    </div>
  )
}

function ExplorerToggle() {
  const sidebarCollapsed = useIde((s) => s.sidebarCollapsed)
  const setSidebarCollapsed = useIde((s) => s.setSidebarCollapsed)
  return (
    <div className="flex shrink-0 items-stretch border-r border-border">
      <button
        type="button"
        onClick={() => setSidebarCollapsed(!sidebarCollapsed)}
        aria-label="Toggle Explorer"
        className={cn(
          'flex items-center gap-1.5 px-3 text-xs font-medium transition-colors hover:text-foreground',
          sidebarCollapsed ? 'text-muted-foreground' : 'text-foreground',
        )}
      >
        <Icon name="sidebar-left-01" size={13} />
        Explorer
      </button>
    </div>
  )
}

function WorkspaceTab({
  workspace,
  active,
  onActivate,
}: {
  workspace: Workspace
  active: boolean
  onActivate: () => void
}) {
  return (
    <button
      type="button"
      onClick={onActivate}
      className={cn(
        'flex h-full shrink-0 items-center border-b-2 px-4 text-xs font-medium transition-colors',
        active
          ? 'border-primary text-foreground'
          : 'border-transparent text-muted-foreground hover:text-foreground',
      )}
    >
      {workspace.name}
    </button>
  )
}

// ─── Surface ───────────────────────────────────────────────────────────────────

function WorkspaceIdeSurface({ orgSlug, workspace }: { orgSlug: string; workspace: Workspace }) {
  const sidebarRef = useRef<PanelImperativeHandle>(null)
  const sidebarCollapsed = useIde((s) => s.sidebarCollapsed)
  const setSidebarCollapsed = useIde((s) => s.setSidebarCollapsed)

  // Sync store → panel (e.g. on initial mount with persisted state)
  useEffect(() => {
    if (sidebarCollapsed) {
      sidebarRef.current?.collapse()
    } else {
      sidebarRef.current?.expand()
    }
  }, [sidebarCollapsed])

  return (
    <ResizablePanelGroup orientation="horizontal" className="min-h-0 flex-1">
      <ResizablePanel
        panelRef={sidebarRef}
        defaultSize="22%"
        minSize="14%"
        maxSize="40%"
        collapsible
        collapsedSize="0%"
        className="overflow-hidden"
        onResize={(size) => setSidebarCollapsed(size.asPercentage === 0)}
      >
        <IdeSidebar orgSlug={orgSlug} workspace={workspace} />
      </ResizablePanel>

      <ResizableHandle withHandle />

      <ResizablePanel defaultSize="78%" minSize="45%" className="overflow-hidden">
        <IdeEditorAndResults orgSlug={orgSlug} workspace={workspace} />
      </ResizablePanel>
    </ResizablePanelGroup>
  )
}

// ─── Sidebar ───────────────────────────────────────────────────────────────────

function IdeSidebar({ orgSlug, workspace }: { orgSlug: string; workspace: Workspace }) {
  const dbRef = useRef<PanelImperativeHandle>(null)
  const filesRef = useRef<PanelImperativeHandle>(null)
  const maximizedSidebarPane = useIde((s) => s.maximizedSidebarPane)
  const setMaximizedSidebarPane = useIde((s) => s.setMaximizedSidebarPane)

  function handleDbMaximize(maximized: boolean) {
    if (maximized) {
      filesRef.current?.collapse()
      setMaximizedSidebarPane('database')
    } else {
      filesRef.current?.expand()
      setMaximizedSidebarPane(null)
    }
  }

  function handleFilesMaximize(maximized: boolean) {
    if (maximized) {
      dbRef.current?.collapse()
      setMaximizedSidebarPane('files')
    } else {
      dbRef.current?.expand()
      setMaximizedSidebarPane(null)
    }
  }

  return (
    <aside className="flex h-full min-h-0 flex-col border-r border-border bg-sidebar">
      <ResizablePanelGroup orientation="vertical" className="min-h-0">
        <ResizablePanel
          panelRef={dbRef}
          defaultSize="55%"
          minSize="15%"
          collapsible
          collapsedSize="0%"
          className="overflow-hidden"
        >
          <DatabasePanel
            orgSlug={orgSlug}
            workspace={workspace}
            maximized={maximizedSidebarPane === 'database'}
            onMaximizedChange={handleDbMaximize}
          />
        </ResizablePanel>
        <ResizableHandle withHandle />
        <ResizablePanel
          panelRef={filesRef}
          defaultSize="45%"
          minSize="15%"
          collapsible
          collapsedSize="0%"
          className="overflow-hidden"
        >
          <FilesPanel
            orgSlug={orgSlug}
            workspace={workspace}
            maximized={maximizedSidebarPane === 'files'}
            onMaximizedChange={handleFilesMaximize}
          />
        </ResizablePanel>
      </ResizablePanelGroup>
    </aside>
  )
}

// ─── Editor + Results ──────────────────────────────────────────────────────────

function IdeEditorAndResults({ orgSlug, workspace }: { orgSlug: string; workspace: Workspace }) {
  const editorRef = useRef<PanelImperativeHandle>(null)
  const resultsRef = useRef<PanelImperativeHandle>(null)
  const maximizedPane = useIde((s) => s.maximizedPane)

  useEffect(() => {
    if (maximizedPane === 'editor') {
      resultsRef.current?.collapse()
      editorRef.current?.expand()
    } else if (maximizedPane === 'results') {
      editorRef.current?.collapse()
      resultsRef.current?.expand()
    } else {
      editorRef.current?.expand()
      resultsRef.current?.expand()
    }
  }, [maximizedPane])

  return (
    <ResizablePanelGroup orientation="vertical" className="min-h-0 flex-1">
      <ResizablePanel
        panelRef={editorRef}
        defaultSize="58%"
        minSize="15%"
        collapsible
        collapsedSize="0%"
        className="flex min-h-0 flex-col overflow-hidden"
      >
        <EditorSection orgSlug={orgSlug} workspace={workspace} />
      </ResizablePanel>
      <ResizableHandle withHandle />
      <ResizablePanel
        panelRef={resultsRef}
        defaultSize="42%"
        minSize="12%"
        collapsible
        collapsedSize="0%"
        className="flex min-h-0 flex-col overflow-hidden"
      >
        <ResultsArea />
      </ResizablePanel>
    </ResizablePanelGroup>
  )
}

type CursorInfo = { line: number; col: number; sel: number }

function EditorSection({ orgSlug, workspace }: { orgSlug: string; workspace: Workspace }) {
  const registry = useYDocRegistry()
  const queryClient = useQueryClient()
  const [createFileOpen, setCreateFileOpen] = useState(false)
  const [cursorInfo, setCursorInfo] = useState<CursorInfo | null>(null)

  const activeTabId = useIde((s) => s.activeTabIds[workspace.id])
  const tabs = useIde((s) => s.tabs)
  const openConsole = useIde((s) => s.openConsole)
  const openTab = useIde((s) => s.openTab)
  const updateTabContent = useIde((s) => s.updateTabContent)
  const updateTabEtag = useIde((s) => s.updateTabEtag)

  const activeTab = useMemo(
    () => tabs.find((t) => t.id === activeTabId && t.workspaceId === workspace.id),
    [tabs, activeTabId, workspace.id],
  )

  // Stable helper: creates the Y.js initial state and opens a new console.
  const createConsole = useCallback(() => {
    const tmpDoc = new Y.Doc()
    tmpDoc.getText('content').insert(0, DEFAULT_CONSOLE_CONTENT)
    const yState = Array.from(Y.encodeStateAsUpdate(tmpDoc))
    tmpDoc.destroy()
    openConsole(workspace, yState)
  }, [workspace, openConsole])

  // ── Tab → Y.Doc lifecycle ──────────────────────────────────────────────────
  // Use ALL tabs (not just wsTabs) so switching workspaces never destroys
  // Y.Docs for tabs in other workspaces. Docs are only torn down when a tab
  // is explicitly closed (removed from `tabs`).
  const trackedIdsRef = useRef(new Set<string>())
  useEffect(() => {
    const currentIds = new Set(tabs.map((t) => t.id))
    for (const tab of tabs) {
      if (!trackedIdsRef.current.has(tab.id)) {
        // Init priority: ySnapshot > yState > plain-text content
        // ySnapshot: current Y.js state persisted on each debounced write — used
        //   on page reload so console edits and dirty file edits survive refresh.
        // yState: creation-time state — for cross-window sync bootstrapping.
        // content: plain text fallback for connection tabs and legacy scratch tabs.
        const initState = tab.ySnapshot ?? tab.yState
        const doc = registry.getOrCreate(
          tab.id,
          initState ? undefined : tab.kind === 'file' ? undefined : tab.content,
        )
        if (initState && doc.getText('content').length === 0) {
          // ySnapshot → 'init': restoring local state, no broadcast needed.
          // yState only → 'server-load': first open in another window; broadcast
          //   so peers don't need to re-fetch the creation-time state.
          const origin = tab.ySnapshot ? 'init' : 'server-load'
          Y.applyUpdate(doc, new Uint8Array(initState), origin)
        }
      }
    }
    for (const id of trackedIdsRef.current) {
      if (!currentIds.has(id)) registry.destroy(id)
    }
    trackedIdsRef.current = currentIds
  }, [tabs, registry])

  // ── File tab close → remove query cache so reopen always fetches fresh ────
  const prevTabsRef = useRef<EditorTab[]>(tabs)
  useEffect(() => {
    const prevTabs = prevTabsRef.current
    const currIds = new Set(tabs.map((t) => t.id))
    for (const tab of prevTabs) {
      if (!currIds.has(tab.id) && tab.kind === 'file' && tab.fileId != null) {
        queryClient.removeQueries({ queryKey: ['file-content', orgSlug, workspace.id, tab.fileId] })
      }
    }
    prevTabsRef.current = tabs
  }, [tabs, queryClient, orgSlug, workspace.id])

  // ── Y.Doc → store: debounced snapshot for IndexedDB persistence + isDirty ─
  // 'server-load' and 'init' are skipped — they are not user edits.
  // User typing and 'broadcast' (cross-window sync) both update the snapshot
  // and, via updateTabContent's existing isDirty logic, mark the tab dirty
  // when an etag is set.
  // Use ALL tabs so switching workspaces doesn't cancel pending debounce timers
  // for tabs in other workspaces, which would lose uncommitted edits.
  useEffect(() => {
    const cleanups: Array<() => void> = []
    const timers: Record<string, ReturnType<typeof setTimeout>> = {}

    for (const tab of tabs) {
      const doc = registry.get(tab.id)
      if (!doc) continue
      const observer = (_update: Uint8Array, origin: unknown) => {
        if (origin === 'server-load' || origin === 'init') return
        clearTimeout(timers[tab.id])
        timers[tab.id] = setTimeout(() => {
          const content = doc.getText('content').toString()
          const snapshot = Array.from(Y.encodeStateAsUpdate(doc))
          updateTabContent(tab.id, content, snapshot)
        }, 400)
      }
      doc.on('update', observer)
      cleanups.push(() => {
        doc.off('update', observer)
        clearTimeout(timers[tab.id])
      })
    }

    return () => cleanups.forEach((c) => c())
  }, [tabs, registry, updateTabContent])

  // Reset cursor info when the active tab changes.
  useEffect(() => { setCursorInfo(null) }, [activeTabId])

  // No "ensure one tab" guard — users can close all tabs to reach the empty state.

  // ── ⌘S / Ctrl+S: save file tab in-place ────────────────────────────────────
  useEffect(() => {
    async function handleKeyDown(e: KeyboardEvent) {
      if (!(e.metaKey || e.ctrlKey) || e.key !== 's') return
      e.preventDefault()
      if (!activeTab || activeTab.kind !== 'file' || !activeTab.etag || !activeTab.fileId) return
      // Read from Y.Doc — more current than the 400 ms debounced snapshot.
      const doc = registry.get(activeTab.id)
      const content = doc ? doc.getText('content').toString() : activeTab.content
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
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [activeTab, orgSlug, workspace.id, updateTabEtag, registry])

  // ── File content loading ───────────────────────────────────────────────────
  const { isLoading: isContentLoading, isError: isContentError } = useFileContent({
    orgSlug,
    workspaceId: workspace.id,
    tab: activeTab,
    updateTabEtag,
  })

  // ── Render ─────────────────────────────────────────────────────────────────
  // Populate the Y.Doc synchronously in render so SqlEditor always mounts with
  // content. React runs child effects (SqlEditor's) before parent effects
  // (EditorSection's), so waiting for the lifecycle useEffect is too late —
  // EditorView.create() captures yText.toString() at mount time.
  let activeDoc: Y.Doc | undefined
  if (activeTab) {
    const initState = activeTab.ySnapshot ?? activeTab.yState
    const initialContent = !initState && activeTab.kind !== 'file'
      ? activeTab.content
      : undefined
    activeDoc = registry.getOrCreate(activeTab.id, initialContent)
    if (initState && activeDoc.getText('content').length === 0) {
      Y.applyUpdate(activeDoc, new Uint8Array(initState), 'init')
    }
  }

  return (
    <>
    <section className="flex h-full min-h-0 flex-col bg-background">
      <IdeToolbar orgSlug={orgSlug} workspace={workspace} />
      <IdeTabBar orgSlug={orgSlug} workspace={workspace} />
      <div className="min-h-0 flex-1 border-t border-border bg-card">
        {activeTab && activeDoc ? (
          isContentLoading ? (
            <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
              Loading…
            </div>
          ) : isContentError ? (
            <div className="flex h-full items-center justify-center text-xs text-destructive">
              Failed to load file content.
            </div>
          ) : (
            <SqlEditor
              key={activeTab.id}
              tabId={activeTab.id}
              doc={activeDoc}
              className="h-full"
              onCursorChange={(line, col, sel) => setCursorInfo({ line, col, sel })}
            />
          )
        ) : (
          <EmptyEditorState
            onNewConsole={createConsole}
            onNewFile={() => setCreateFileOpen(true)}
          />
        )}
      </div>
      <EditorStatusBar cursorInfo={cursorInfo} hasActiveTab={!!activeTab} />
    </section>

    <SaveAsDialog
      open={createFileOpen}
      onOpenChange={(open) => { if (!open) setCreateFileOpen(false) }}
      tab={{ id: 'new-empty', workspaceId: workspace.id, title: 'untitled', kind: 'scratch', content: '' }}
      orgSlug={orgSlug}
      workspaceId={workspace.id}
      onSuccess={(file, etag) => {
        openTab({
          id: `file:${file.id}`,
          workspaceId: workspace.id,
          title: file.name,
          kind: 'file',
          subtitle: file.name,
          fileId: file.id,
          content: '',
          etag,
          isDirty: false,
        })
      }}
    />
    </>
  )
}

// ─── Editor status bar ─────────────────────────────────────────────────────────

function EditorStatusBar({ cursorInfo, hasActiveTab }: { cursorInfo: CursorInfo | null; hasActiveTab: boolean }) {
  return (
    <div className="flex h-5 shrink-0 items-center border-t border-border bg-muted/30 px-3 text-[10px] text-muted-foreground">
      {hasActiveTab && cursorInfo && (
        <>
          <span className="tabular-nums">Ln {cursorInfo.line}, Col {cursorInfo.col}</span>
          {cursorInfo.sel > 0 && (
            <span className="ml-2 tabular-nums">({cursorInfo.sel} selected)</span>
          )}
        </>
      )}
      <div className="flex-1" />
      {hasActiveTab && <span>SQL</span>}
    </div>
  )
}

// ─── Empty state ───────────────────────────────────────────────────────────────

type EmptyEditorStateProps = {
  onNewConsole: () => void
  onNewFile: () => void
}

function EmptyEditorState({ onNewConsole, onNewFile }: EmptyEditorStateProps) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-8 p-8">
      <div className="text-center">
        <p className="text-sm font-semibold text-foreground">No editors open</p>
        <p className="mt-1 text-xs text-muted-foreground">Start by opening a console or a file.</p>
      </div>

      <div className="flex gap-3">
        <EmptyStateCard
          icon="terminal"
          title="New Console"
          description="Write and run SQL queries against a connection"
          onClick={onNewConsole}
        />
        <EmptyStateCard
          icon="file-01"
          title="New File"
          description="Create a reusable SQL script saved to this workspace"
          onClick={onNewFile}
        />
      </div>

      <p className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
        <Icon name="folder-open" size={12} className="shrink-0" />
        To open an existing file, browse the
        <span className="font-medium text-foreground">Files</span>
        panel in the sidebar.
      </p>
    </div>
  )
}

function EmptyStateCard({
  icon,
  title,
  description,
  onClick,
}: {
  icon: import('#/lib/icons').AppIcon
  title: string
  description: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'group flex w-44 flex-col items-center gap-3 rounded-lg p-5 text-center',
        'ring-1 ring-foreground/10 transition-all',
        'hover:bg-accent hover:ring-foreground/20',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
      )}
    >
      <div className={cn(
        'flex h-10 w-10 items-center justify-center rounded-md',
        'bg-muted transition-colors group-hover:bg-background',
      )}>
        <Icon name={icon} size={18} className="text-muted-foreground group-hover:text-foreground transition-colors" />
      </div>
      <div className="flex flex-col gap-1">
        <p className="text-xs font-medium text-foreground">{title}</p>
        <p className="text-[11px] leading-relaxed text-muted-foreground">{description}</p>
      </div>
    </button>
  )
}

// ─── Utility ───────────────────────────────────────────────────────────────────

function IdeFrame({ children }: { children: React.ReactNode }) {
  return (
    <div className="-mx-4 -my-6 flex h-svh items-center justify-center bg-background text-sm text-muted-foreground md:-mx-6">
      {children}
    </div>
  )
}
