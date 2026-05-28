import { useEffect, useMemo, useRef } from 'react'
import { useQuery } from '@tanstack/react-query'
import { HugeiconsIcon } from '@hugeicons/react'
import { DatabaseLightningIcon } from '@hugeicons/core-free-icons'
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
  newScratchTab,
} from './useIdeStore'
import { DatabasePanel } from './DatabasePanel'
import { FilesPanel } from './FilesPanel'
import { IdeToolbar } from './IdeToolbar'
import { IdeTabBar } from './IdeTabBar'
import { SqlEditor } from './SqlEditor'
import { ResultsArea } from './ResultsArea'

// ─── Root ──────────────────────────────────────────────────────────────────────

type WorkspaceIdeProps = { orgSlug: string }

export function WorkspaceIde({ orgSlug }: WorkspaceIdeProps) {
  const store = useMemo(() => createIdeStore(orgSlug), [orgSlug])
  const workspaces = useQuery(
    orgWorkspacesQueryOptions(orgSlug, { page_size: 100, sort: 'name', order: 'asc' }),
  )

  if (workspaces.isLoading) return <IdeFrame>Loading workspaces…</IdeFrame>
  if (workspaces.isError) return <IdeFrame>Unable to load workspaces.</IdeFrame>

  const items = workspaces.data?.items ?? []
  if (items.length === 0) return <IdeFrame>No accessible workspaces.</IdeFrame>

  return (
    <IdeStoreContext.Provider value={store}>
      <WorkspaceIdeInner orgSlug={orgSlug} workspaces={items} />
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
      {/* Top bar: logo icon + workspace tabs */}
      <div className="flex h-10 shrink-0 items-stretch border-b border-border">
        <div className="flex w-10 shrink-0 items-center justify-center border-r border-border">
          <HugeiconsIcon icon={DatabaseLightningIcon} size={16} strokeWidth={2} className="text-primary" />
        </div>
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
  return (
    <ResizablePanelGroup orientation="horizontal" className="min-h-0 flex-1">
      <ResizablePanel defaultSize={22} minSize={14} maxSize={38} className="overflow-hidden">
        <IdeSidebar orgSlug={orgSlug} workspace={workspace} />
      </ResizablePanel>
      <ResizableHandle withHandle />
      <ResizablePanel defaultSize={78} minSize={45} className="overflow-hidden">
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
          defaultSize={55}
          minSize={15}
          collapsible
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
          defaultSize={45}
          minSize={15}
          collapsible
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
        defaultSize={58}
        minSize={15}
        collapsible
        className="flex min-h-0 flex-col overflow-hidden"
      >
        <EditorSection orgSlug={orgSlug} workspace={workspace} />
      </ResizablePanel>
      <ResizableHandle withHandle />
      <ResizablePanel
        panelRef={resultsRef}
        defaultSize={42}
        minSize={12}
        collapsible
        className="flex min-h-0 flex-col overflow-hidden"
      >
        <ResultsArea />
      </ResizablePanel>
    </ResizablePanelGroup>
  )
}

function EditorSection({ orgSlug, workspace }: { orgSlug: string; workspace: Workspace }) {
  const activeTabId = useIde((s) => s.activeTabId)
  const tabs = useIde((s) => s.tabs)
  const openTab = useIde((s) => s.openTab)
  const updateTabContent = useIde((s) => s.updateTabContent)

  const activeTab = tabs.find((t) => t.id === activeTabId)

  // Auto-open a scratch tab if this workspace has none
  useEffect(() => {
    const wsTabCount = tabs.filter((t) => t.workspaceId === workspace.id).length
    if (wsTabCount === 0) {
      openTab(newScratchTab(workspace))
    }
  }, [workspace, tabs, openTab])

  return (
    <section className="flex h-full min-h-0 flex-col bg-background">
      <IdeToolbar orgSlug={orgSlug} workspace={workspace} />
      <IdeTabBar workspace={workspace} />
      <div className="min-h-0 flex-1 bg-card">
        {activeTab ? (
          <SqlEditor
            key={activeTab.id}
            value={activeTab.content}
            onChange={(content) => updateTabContent(activeTab.id, content)}
            className="h-full"
          />
        ) : (
          <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
            Open a connection or file to start querying.
          </div>
        )}
      </div>
    </section>
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
