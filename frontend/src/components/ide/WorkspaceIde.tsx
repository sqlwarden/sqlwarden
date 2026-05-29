import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import * as Y from 'yjs'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  DatabaseLightningIcon,
  File01Icon,
  FolderOpenIcon,
  SidebarLeft01Icon,
  TerminalIcon,
} from '@hugeicons/core-free-icons'
import { Button } from '#/components/ui/button'
import { updatePrivateWorkspaceFileContent } from '#/lib/api/files'
import type { PanelImperativeHandle } from 'react-resizable-panels'
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from '#/components/ui/resizable'
import { orgWorkspacesQueryOptions } from '#/lib/api/query'
import type { Workspace } from '#/lib/api/types'
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

// ─── Root ──────────────────────────────────────────────────────────────────────

type WorkspaceIdeProps = { orgSlug: string }

export function WorkspaceIde({ orgSlug }: WorkspaceIdeProps) {
  const store = useMemo(() => createIdeStore(orgSlug), [orgSlug])
  const registry = useMemo(() => createYDocRegistry(), [orgSlug])

  // ── Cross-window etag / dirty-state sync ─────────────────────────────────
  // When any window saves a file (etag changes) or completes a server load
  // (initial etag set), broadcast the new etag to all other windows on the
  // same origin so their dirty indicator and save ETag stay in sync.
  // The receiving window calls updateTabEtag directly — no re-broadcast.
  useEffect(() => {
    const channel = new BroadcastChannel(`sqlwarden:store:${orgSlug}`)
    const prevEtags = new Map<string, string>()
    const prevTabIds = new Set<string>()
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
      }

      // Always update prev state (even when applyingRemote) to stay current.
      prevTabIds.clear()
      currentTabIds.forEach((id) => prevTabIds.add(id))
      for (const tab of state.tabs) {
        if (tab.etag !== undefined) prevEtags.set(tab.id, tab.etag)
      }
    })

    channel.addEventListener('message', handleRemote)
    return () => {
      unsub()
      channel.removeEventListener('message', handleRemote)
      channel.close()
    }
  }, [store, orgSlug])

  const workspaces = useQuery(
    orgWorkspacesQueryOptions(orgSlug, { page_size: 100, sort: 'name', order: 'asc' }),
  )

  if (workspaces.isLoading) return <IdeFrame>Loading workspaces…</IdeFrame>
  if (workspaces.isError) return <IdeFrame>Unable to load workspaces.</IdeFrame>

  const items = workspaces.data?.items ?? []
  if (items.length === 0) return <IdeFrame>No accessible workspaces.</IdeFrame>

  return (
    <IdeStoreContext.Provider value={store}>
      <YDocRegistryContext.Provider value={registry}>
        <WorkspaceIdeInner orgSlug={orgSlug} workspaces={items} />
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
      {/* Top bar: logo + explorer toggle + workspace tabs */}
      <div className="flex h-10 shrink-0 items-stretch border-b border-border">
        <div className="flex w-10 shrink-0 items-center justify-center border-r border-border">
          <HugeiconsIcon icon={DatabaseLightningIcon} size={16} strokeWidth={2} className="text-primary" />
        </div>
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
        <HugeiconsIcon icon={SidebarLeft01Icon} size={13} strokeWidth={2} />
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
        onCollapse={() => setSidebarCollapsed(true)}
        onExpand={() => setSidebarCollapsed(false)}
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

function EditorSection({ orgSlug, workspace }: { orgSlug: string; workspace: Workspace }) {
  const registry = useYDocRegistry()
  const [createFileOpen, setCreateFileOpen] = useState(false)

  const activeTabId = useIde((s) => s.activeTabId)
  const tabs = useIde((s) => s.tabs)
  const openConsole = useIde((s) => s.openConsole)
  const openTab = useIde((s) => s.openTab)
  const updateTabContent = useIde((s) => s.updateTabContent)
  const updateTabEtag = useIde((s) => s.updateTabEtag)

  const activeTab = useMemo(() => tabs.find((t) => t.id === activeTabId), [tabs, activeTabId])
  const wsTabs = useMemo(
    () => tabs.filter((t) => t.workspaceId === workspace.id),
    [tabs, workspace.id],
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
  const trackedIdsRef = useRef(new Set<string>())
  useEffect(() => {
    const currentIds = new Set(wsTabs.map((t) => t.id))
    for (const tab of wsTabs) {
      if (!trackedIdsRef.current.has(tab.id)) {
        // All tabs start with an empty Y.Doc.
        // File tabs: useFileContent populates via server fetch.
        // Scratch tabs with yState: applied below via 'server-load' (broadcasts to peers).
        // Connection tabs / legacy scratch (no yState): seed from tab.content via 'init'.
        const doc = registry.getOrCreate(
          tab.id,
          tab.yState ? undefined : tab.kind === 'file' ? undefined : tab.content,
        )
        if (tab.yState && doc.getText('content').length === 0) {
          // Apply canonical initial state so all windows share identical Y.js history.
          // 'server-load' origin broadcasts the full state so peers that joined
          // before us can sync without making a second request.
          Y.applyUpdate(doc, new Uint8Array(tab.yState), 'server-load')
        }
      }
    }
    for (const id of trackedIdsRef.current) {
      if (!currentIds.has(id)) registry.destroy(id)
    }
    trackedIdsRef.current = currentIds
  }, [wsTabs, registry])

  // ── Y.Doc → store: debounced snapshot for IndexedDB persistence + isDirty ─
  // 'server-load' and 'init' are skipped — they are not user edits.
  // User typing and 'broadcast' (cross-window sync) both update the snapshot
  // and, via updateTabContent's existing isDirty logic, mark the tab dirty
  // when an etag is set.
  useEffect(() => {
    const cleanups: Array<() => void> = []
    const timers: Record<string, ReturnType<typeof setTimeout>> = {}

    for (const tab of wsTabs) {
      const doc = registry.get(tab.id)
      if (!doc) continue
      const observer = (_update: Uint8Array, origin: unknown) => {
        if (origin === 'server-load' || origin === 'init') return
        clearTimeout(timers[tab.id])
        timers[tab.id] = setTimeout(() => {
          updateTabContent(tab.id, doc.getText('content').toString())
        }, 400)
      }
      doc.on('update', observer)
      cleanups.push(() => {
        doc.off('update', observer)
        clearTimeout(timers[tab.id])
      })
    }

    return () => cleanups.forEach((c) => c())
  }, [wsTabs, registry, updateTabContent])

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
  // getOrCreate is idempotent — safe to call in render. This ensures the
  // Y.Doc exists immediately after Zustand rehydrates from IndexedDB, so
  // the editor mounts without waiting for the lifecycle useEffect to run.
  // The lifecycle effect still handles yState, the observer, and cleanup.
  const activeDoc = activeTab ? registry.getOrCreate(activeTab.id) : undefined

  return (
    <>
    <section className="flex h-full min-h-0 flex-col bg-background">
      <IdeToolbar orgSlug={orgSlug} workspace={workspace} />
      <IdeTabBar orgSlug={orgSlug} workspace={workspace} />
      <div className="min-h-0 flex-1 bg-card">
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
            <SqlEditor key={activeTab.id} doc={activeDoc} className="h-full" />
          )
        ) : (
          <EmptyEditorState
            onNewConsole={createConsole}
            onNewFile={() => setCreateFileOpen(true)}
          />
        )}
      </div>
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
          icon={TerminalIcon}
          title="New Console"
          description="Write and run SQL queries against a connection"
          onClick={onNewConsole}
        />
        <EmptyStateCard
          icon={File01Icon}
          title="New File"
          description="Create a reusable SQL script saved to this workspace"
          onClick={onNewFile}
        />
      </div>

      <p className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
        <HugeiconsIcon icon={FolderOpenIcon} size={12} strokeWidth={2} className="shrink-0" />
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
  icon: React.ComponentType<React.SVGProps<SVGSVGElement>>
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
        <HugeiconsIcon icon={icon} size={18} strokeWidth={1.5} className="text-muted-foreground group-hover:text-foreground transition-colors" />
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
