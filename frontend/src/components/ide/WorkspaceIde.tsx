import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import * as Y from 'yjs'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { useBrand } from '#/lib/brand/brand'
import type { PanelImperativeHandle } from 'react-resizable-panels'
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '#/components/ui/resizable'
import { Button } from '#/components/ui/button'
import { useIsMobile } from '#/hooks/use-mobile'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '#/components/ui/sheet'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '#/components/ui/empty'
import { Skeleton } from '#/components/ui/skeleton'
import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import {
  orgWorkspacesQueryOptions,
  orgEffectivePermissionsQueryOptions,
  orgRuntimeSettingsQueryOptions,
  allOrgWorkspaceConnectionsQueryOptions,
} from '#/lib/api/query'
import { resolveDeepLink } from './ideDeepLink'
import { hasAnyPermission, permission } from '#/lib/permissions'
import { IdeTopBarControls } from './IdeTopBarControls'
import { IdeActivityBar } from './IdeActivityBar'
import { visibleActivities, type ActivityVisibilityContext } from './ideActivities'
import type { Workspace } from '#/lib/api/types'
import { useSession } from '#/hooks/use-session'
import { cn } from '#/lib/utils'
import { usePageTitle } from '#/lib/page-title'
import { ContextMenu, ContextMenuProvider } from '#/components/ui/context-menu'
import { buildWorkspaceMenu } from './contextMenus/workspaceMenu'
import {
  createIdeStore,
  IdeStoreContext,
  useIde,
  activeTabId as selectActiveTabId,
  DEFAULT_CONSOLE_CONTENT,
} from './useIdeStore'
import { SaveAsDialog } from './SaveAsDialog'
import { IdeToolbar } from './IdeToolbar'
import { EditorLayout } from './EditorLayout'
import { ResultsArea } from './ResultsArea'
import { createYDocRegistry, YDocRegistryContext, useYDocRegistry } from './useYDocRegistry'
import { createEditorViewRegistry, EditorViewRegistryContext } from './useEditorViewRegistry'
import { Tip } from './schema-diagram/Tip'
import { useSessionSync } from './useSessionSync'
import { useSaveEditorTab } from './useSaveEditorTab'
import { createStoreSync } from './storeSync'
import {
  useClosedFileCacheCleanup,
  useEditorDocumentLifecycle,
  useEditorSaveShortcut,
  useEditorSnapshotPersistence,
} from './useEditorDocumentLifecycle'

// ─── Root ──────────────────────────────────────────────────────────────────────

type WorkspaceIdeProps = { orgSlug: string }

export function WorkspaceIde({ orgSlug }: WorkspaceIdeProps) {
  // Parent route guards that session is loaded before rendering this component,
  // so session.data is always available here (cache hit, no network request).
  // Fall back to 0 defensively so hooks are never called with undefined deps.
  const { data: session } = useSession()
  const accountId = session?.account?.id ?? 0

  const store = useMemo(() => createIdeStore(orgSlug, accountId), [orgSlug, accountId])
  const registry = useMemo(() => createYDocRegistry(accountId, orgSlug), [orgSlug, accountId])
  const viewRegistry = useMemo(() => createEditorViewRegistry(), [])

  // Release the primary lock when this editor window unmounts so another window can
  // take over persistence.
  useEffect(() => {
    return () => {
      ;(store as unknown as { __cleanupElection?: () => void }).__cleanupElection?.()
      registry.disposeAll()
    }
  }, [store, registry])

  useEffect(() => {
    const channel = new BroadcastChannel(`sqlwarden:store:${orgSlug}:${accountId}`)
    return createStoreSync(store, channel)
  }, [store, orgSlug, accountId])

  // Abort all in-flight queries when the editor unmounts (e.g. navigation away).
  useEffect(() => {
    return () => {
      const { abortControllers } = store.getState()
      Object.values(abortControllers).forEach((c) => c.abort())
    }
  }, [store])

  const workspaces = useQuery(
    orgWorkspacesQueryOptions(orgSlug, { page_size: 100, sort: 'name', order: 'asc' }),
  )

  // Always render providers so useIde never runs without context, even when
  // the workspaces query is still loading or the component suspends mid-render.
  return (
    <IdeStoreContext.Provider value={store}>
      <YDocRegistryContext.Provider value={registry}>
        <EditorViewRegistryContext.Provider value={viewRegistry}>
          <WorkspaceIdeContent
            orgSlug={orgSlug}
            isLoading={workspaces.isLoading}
            isError={workspaces.isError}
            isRetrying={workspaces.isFetching}
            workspaces={workspaces.data?.items ?? []}
            onRetry={() => {
              void workspaces.refetch()
            }}
          />
        </EditorViewRegistryContext.Provider>
      </YDocRegistryContext.Provider>
    </IdeStoreContext.Provider>
  )
}

type WorkspaceIdeContentProps = {
  orgSlug: string
  isLoading: boolean
  isError: boolean
  isRetrying: boolean
  workspaces: Workspace[]
  onRetry: () => void
}

export function WorkspaceIdeContent({
  orgSlug,
  isLoading,
  isError,
  isRetrying,
  workspaces,
  onRetry,
}: WorkspaceIdeContentProps) {
  if (isLoading) return <WorkspaceIdeSkeleton />
  if (isError) return <WorkspaceLoadError isRetrying={isRetrying} onRetry={onRetry} />
  if (workspaces.length === 0) return <NoWorkspaceAccess />
  return <WorkspaceIdeInner orgSlug={orgSlug} workspaces={workspaces} />
}

export function WorkspaceIdeSkeleton() {
  usePageTitle('Editor')

  return (
    <main
      aria-busy="true"
      aria-live="polite"
      className="flex h-dvh w-dvw flex-col overflow-hidden bg-background"
      role="status"
    >
      <span className="sr-only">Loading editor…</span>
      <div aria-hidden="true" className="flex min-h-0 flex-1 flex-col">
        <div className="flex h-10 shrink-0 items-center border-b border-border bg-sidebar">
          <div className="flex h-full w-36 shrink-0 items-center gap-2 border-r border-border px-3">
            <Skeleton className="size-5 rounded-sm" />
            <Skeleton className="h-3.5 w-20" />
          </div>
          <div className="flex min-w-0 flex-1 items-center gap-1 px-2">
            <Skeleton className="h-7 w-28 rounded-sm" />
            <Skeleton className="h-7 w-24 rounded-sm" />
          </div>
          <div className="flex shrink-0 items-center gap-2 px-3">
            <Skeleton className="size-6 rounded-sm" />
            <Skeleton className="size-6 rounded-full" />
          </div>
        </div>

        <div className="flex min-h-0 flex-1">
          <div className="flex w-10 shrink-0 flex-col items-center gap-3 border-r border-border bg-sidebar py-3">
            {Array.from({ length: 4 }).map((_, index) => (
              <Skeleton key={index} className="size-5 rounded-sm" />
            ))}
          </div>

          <div className="hidden w-60 shrink-0 flex-col border-r border-border bg-sidebar sm:flex">
            <div className="flex h-9 shrink-0 items-center border-b border-border px-3">
              <Skeleton className="h-3.5 w-20" />
            </div>
            <div className="flex flex-col gap-3 p-3">
              <Skeleton className="h-3 w-3/4" />
              <Skeleton className="ms-2 h-3 w-2/3" />
              <Skeleton className="ms-4 h-3 w-3/4" />
              <Skeleton className="ms-4 h-3 w-1/2" />
              <Skeleton className="h-3 w-4/5" />
              <Skeleton className="ms-2 h-3 w-3/5" />
              <Skeleton className="ms-4 h-3 w-2/3" />
            </div>
          </div>

          <div className="flex min-h-0 min-w-0 flex-1 flex-col">
            <div className="flex h-9 shrink-0 items-center gap-2 border-b border-border px-3">
              <Skeleton className="h-3.5 w-32" />
              <Skeleton className="h-3.5 w-24" />
            </div>
            <div className="flex min-h-0 flex-1 flex-col gap-3 p-4">
              <Skeleton className="h-3.5 w-2/3" />
              <Skeleton className="h-3.5 w-1/2" />
              <Skeleton className="h-3.5 w-3/4" />
              <Skeleton className="h-3.5 w-2/5" />
              <Skeleton className="h-3.5 w-3/5" />
            </div>
            <div className="flex h-48 shrink-0 flex-col border-t border-border">
              <div className="flex h-8 shrink-0 items-center gap-4 border-b border-border px-3">
                <Skeleton className="h-3 w-20" />
                <Skeleton className="h-3 w-16" />
                <Skeleton className="h-3 w-24" />
              </div>
              <div className="grid grid-cols-4 gap-x-4 gap-y-3 p-3">
                {Array.from({ length: 12 }).map((_, index) => (
                  <Skeleton key={index} className="h-3 w-full" />
                ))}
              </div>
            </div>
          </div>
        </div>
      </div>
    </main>
  )
}

function NoWorkspaceAccess() {
  usePageTitle('Editor')

  return (
    <WorkspaceStateFrame>
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <Icon name="briefcase-01" size={16} />
          </EmptyMedia>
          <EmptyTitle aria-level={1} role="heading">
            No workspace access
          </EmptyTitle>
          <EmptyDescription>
            You don&apos;t currently have access to a workspace in this organization. Ask an
            organization administrator to grant you access.
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    </WorkspaceStateFrame>
  )
}

function WorkspaceLoadError({ isRetrying, onRetry }: { isRetrying: boolean; onRetry: () => void }) {
  usePageTitle('Editor')

  return (
    <WorkspaceStateFrame>
      <Empty>
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <Icon name="refresh" size={16} />
          </EmptyMedia>
          <EmptyTitle aria-level={1} role="heading">
            Unable to load workspaces
          </EmptyTitle>
          <EmptyDescription>
            SQLWarden couldn&apos;t load your workspaces. Check your connection and try again.
          </EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button disabled={isRetrying} onClick={onRetry}>
            <Icon
              className={cn(isRetrying && 'animate-spin motion-reduce:animate-none')}
              data-icon="inline-start"
              name="refresh"
              size={16}
            />
            {isRetrying ? 'Retrying…' : 'Retry'}
          </Button>
        </EmptyContent>
      </Empty>
    </WorkspaceStateFrame>
  )
}

function WorkspaceStateFrame({ children }: { children: React.ReactNode }) {
  return <main className="flex h-dvh w-dvw overflow-hidden bg-background">{children}</main>
}

// ─── Inner ─────────────────────────────────────────────────────────────────────

function WorkspaceIdeInner({ orgSlug, workspaces }: { orgSlug: string; workspaces: Workspace[] }) {
  const { activeWorkspace, setActiveWorkspace } = useWorkspaceSelection(workspaces)
  const { data: session } = useSession()

  return (
    <>
      <WorkspaceDocumentTitle workspace={activeWorkspace} />
      <WorkspaceIdeInnerContent
        orgSlug={orgSlug}
        workspaces={workspaces}
        activeWorkspace={activeWorkspace}
        setActiveWorkspace={setActiveWorkspace}
        session={session}
      />
    </>
  )
}

export function WorkspaceDocumentTitle({ workspace }: { workspace?: Workspace }) {
  const activeTabId = useIde((state) =>
    workspace ? selectActiveTabId(state, workspace.id) : undefined,
  )
  const activeTabTitle = useIde((state) => state.tabs.find((tab) => tab.id === activeTabId)?.title)
  usePageTitle(
    activeTabTitle ?? workspace?.name ?? 'Editor',
    activeTabTitle ? workspace?.name : workspace ? 'Editor' : undefined,
  )

  return null
}

function WorkspaceIdeInnerContent({
  orgSlug,
  workspaces,
  activeWorkspace,
  setActiveWorkspace,
  session,
}: {
  orgSlug: string
  workspaces: Workspace[]
  activeWorkspace?: Workspace
  setActiveWorkspace: (workspaceId: number) => void
  session: ReturnType<typeof useSession>['data']
}) {
  const orgPermissions = useQuery({
    ...orgEffectivePermissionsQueryOptions(orgSlug, 'org'),
    enabled: Boolean(session),
  })
  const canAccessOrgSettings = hasAnyPermission(orgPermissions.data?.permissions, [
    permission.orgRead,
  ])

  useIdeDeepLink(orgSlug, workspaces)

  return (
    <ContextMenuProvider>
      <div className="flex h-dvh min-h-0 w-dvw max-w-dvw flex-col overflow-hidden bg-background">
        {/* Top bar: brand + explorer toggle + workspace tabs + user controls */}
        <div className="flex h-10 shrink-0 items-stretch border-b border-border bg-sidebar">
          <IdeBrand />
          <div className="flex min-w-0 flex-1 items-stretch overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
            {workspaces.map((ws) => (
              <WorkspaceTab
                key={ws.id}
                orgSlug={orgSlug}
                workspace={ws}
                active={ws.id === activeWorkspace?.id}
                onActivate={() => setActiveWorkspace(ws.id)}
              />
            ))}
          </div>
          {session ? (
            <IdeTopBarControls
              orgSlug={orgSlug}
              session={session}
              canAccessOrgSettings={canAccessOrgSettings}
            />
          ) : null}
        </div>

        {activeWorkspace && <WorkspaceIdeSurface orgSlug={orgSlug} workspace={activeWorkspace} />}
      </div>
    </ContextMenuProvider>
  )
}

export function useWorkspaceSelection(workspaces: Workspace[]) {
  const activeWorkspaceId = useIde((s) => s.activeWorkspaceId)
  const setActiveWorkspace = useIde((s) => s.setActiveWorkspace)

  useEffect(() => {
    if (
      workspaces.length > 0 &&
      !workspaces.some((workspace) => workspace.id === activeWorkspaceId)
    ) {
      setActiveWorkspace(workspaces[0].id)
    }
  }, [activeWorkspaceId, workspaces, setActiveWorkspace])

  return {
    activeWorkspace:
      workspaces.find((workspace) => workspace.id === activeWorkspaceId) ?? workspaces[0],
    setActiveWorkspace,
  }
}

/** Applies ?ws=&conn= once on mount, then strips the params with a replace
 *  navigation so refreshes and cross-window sync don't re-force the
 *  selection. Called after the default-workspace effect in WorkspaceIdeInner
 *  so its setActiveWorkspace wins the initial render. */
function useIdeDeepLink(orgSlug: string, workspaces: Workspace[]) {
  const search = useSearch({ from: '/ide/$org_slug' })
  const navigate = useNavigate()
  const setActiveWorkspace = useIde((s) => s.setActiveWorkspace)
  const setNodeExpanded = useIde((s) => s.setNodeExpanded)

  const hasParams = search.ws !== undefined || search.conn !== undefined
  const needsConnections = search.ws !== undefined && search.conn !== undefined
  const connections = useQuery({
    ...allOrgWorkspaceConnectionsQueryOptions(orgSlug, search.ws ?? 0),
    enabled: needsConnections,
  })

  useEffect(() => {
    if (!hasParams) return

    const resolution = resolveDeepLink(search, workspaces, connections.data?.items)
    if (!resolution.ready && !connections.isError) return

    if (resolution.activateWorkspaceId !== undefined) {
      setActiveWorkspace(resolution.activateWorkspaceId)
    }
    for (const key of resolution.expandKeys) {
      setNodeExpanded(key, true)
    }
    void navigate({
      to: '/ide/$org_slug',
      params: { org_slug: orgSlug },
      search: {},
      replace: true,
    })
  }, [
    hasParams,
    search,
    workspaces,
    connections.data,
    connections.isError,
    setActiveWorkspace,
    setNodeExpanded,
    navigate,
    orgSlug,
  ])
}

function IdeBrand() {
  const brand = useBrand()
  return (
    <Tip label="Back to dashboard" side="bottom">
      <Link
        to="/"
        className="flex w-11 shrink-0 items-center justify-center text-foreground transition-colors hover:bg-sidebar-accent/50"
        aria-label={`${brand.productName} home`}
      >
        <brand.LogoMark size={20} className="shrink-0" />
      </Link>
    </Tip>
  )
}

function WorkspaceTab({
  orgSlug,
  workspace,
  active,
  onActivate,
}: {
  orgSlug: string
  workspace: Workspace
  active: boolean
  onActivate: () => void
}) {
  const navigate = useNavigate()

  const menuItems = buildWorkspaceMenu({
    onOpenSettings: () =>
      navigate({
        to: '/orgs/$org_slug/workspaces/$workspace_id/settings',
        params: { org_slug: orgSlug, workspace_id: String(workspace.id) },
      }),
    onManageMembers: () =>
      navigate({
        to: '/orgs/$org_slug/workspaces/$workspace_id/users',
        params: { org_slug: orgSlug, workspace_id: String(workspace.id) },
      }),
    onManageAccess: () =>
      navigate({
        to: '/orgs/$org_slug/workspaces/$workspace_id/policies',
        params: { org_slug: orgSlug, workspace_id: String(workspace.id) },
      }),
  })

  return (
    <ContextMenu items={menuItems} className="h-full shrink-0">
      <button
        type="button"
        onClick={onActivate}
        className={cn(
          'relative flex h-full shrink-0 items-center px-4 text-xs font-medium transition-colors',
          active
            ? 'bg-background text-foreground after:absolute after:inset-x-0 after:top-0 after:h-[2px] after:bg-primary'
            : 'text-muted-foreground hover:bg-sidebar-accent/50 hover:text-foreground',
        )}
      >
        {workspace.name}
      </button>
    </ContextMenu>
  )
}

// ─── Surface ───────────────────────────────────────────────────────────────────

function WorkspaceIdeSurface({ orgSlug, workspace }: { orgSlug: string; workspace: Workspace }) {
  const sidebarRef = useRef<PanelImperativeHandle>(null)
  const sidebarCollapsed = useIde((s) => s.sidebarCollapsed)
  const setSidebarCollapsed = useIde((s) => s.setSidebarCollapsed)
  const activeActivityId = useIde((s) => s.activeActivityId)
  const isMobile = useIsMobile()

  // Reconcile persisted sessions with the backend for as long as the editor is
  // open — regardless of which sidebar activity is visible.
  useSessionSync(orgSlug, workspace)

  const runtimeSettings = useQuery(orgRuntimeSettingsQueryOptions(orgSlug))
  const visibilityContext: ActivityVisibilityContext = {
    queryHistoryMode: runtimeSettings.data?.effective.query_history_mode ?? 'backend',
    queryFavoritesMode: runtimeSettings.data?.effective.query_favorites_mode ?? 'backend',
  }
  const activities = visibleActivities(visibilityContext)
  const activeActivity = activities.find((a) => a.id === activeActivityId) ?? activities[0]

  // sidebarCollapsed is shared with the mobile Sheet's open/closed state (it
  // doubles as the drawer's persisted "open" flag so IdeActivityBar's toggle
  // works on both layouts), but it's also the persisted desktop panel
  // preference. Track the last desktop value here so a mobile visit's drawer
  // open/close activity can't permanently overwrite it.
  const desktopSidebarCollapsedRef = useRef(sidebarCollapsed)
  const wasMobileRef = useRef(false)

  // The store's sidebarCollapsed default is false (a sensible desktop default),
  // so entering mobile needs an explicit collapse to keep the drawer closed on
  // first mount rather than inheriting an open desktop panel. Leaving mobile
  // restores whatever the desktop preference was before that visit, rather
  // than whatever the mobile drawer's open/close activity left behind. Both
  // concerns live in one effect so the "capture the desktop value" branch
  // can't run on the same commit as the "just left mobile" transition and
  // clobber the value with the mobile-forced one before it's read back.
  useEffect(() => {
    if (isMobile) {
      if (!wasMobileRef.current) setSidebarCollapsed(true)
    } else if (wasMobileRef.current) {
      setSidebarCollapsed(desktopSidebarCollapsedRef.current)
    } else {
      desktopSidebarCollapsedRef.current = sidebarCollapsed
    }
    wasMobileRef.current = isMobile
  }, [isMobile, sidebarCollapsed, setSidebarCollapsed])

  // Sync store → panel (e.g. on initial mount with persisted state). No-ops on
  // mobile since the resizable panel isn't mounted there.
  useEffect(() => {
    if (sidebarCollapsed) {
      sidebarRef.current?.collapse()
    } else {
      sidebarRef.current?.expand()
    }
  }, [sidebarCollapsed])

  // Page-mode activity replaces the whole main region (reserved; no v1 activity uses it).
  if (activeActivity && activeActivity.mode === 'page') {
    const PageSurface = activeActivity.component
    return (
      <div className="flex min-h-0 flex-1 overflow-hidden">
        <IdeActivityBar orgSlug={orgSlug} />
        <div className="min-w-0 flex-1 overflow-hidden">
          <PageSurface orgSlug={orgSlug} workspace={workspace} />
        </div>
      </div>
    )
  }

  if (isMobile) {
    return (
      <div className="flex min-h-0 flex-1 overflow-hidden">
        <IdeActivityBar orgSlug={orgSlug} />
        <div className="min-w-0 flex-1 overflow-hidden">
          <IdeEditorAndResults orgSlug={orgSlug} workspace={workspace} />
        </div>
        <Sheet open={!sidebarCollapsed} onOpenChange={(open) => setSidebarCollapsed(!open)}>
          <SheetContent side="left" className="w-[85vw] max-w-sm p-0 sm:max-w-sm">
            <SheetHeader className="sr-only">
              <SheetTitle>{activeActivity?.label ?? 'Explorer'}</SheetTitle>
              <SheetDescription>Editor sidebar panel</SheetDescription>
            </SheetHeader>
            <IdeSidebar orgSlug={orgSlug} workspace={workspace} />
          </SheetContent>
        </Sheet>
      </div>
    )
  }

  return (
    <div className="flex min-h-0 flex-1 overflow-hidden">
      <IdeActivityBar orgSlug={orgSlug} />
      <ResizablePanelGroup orientation="horizontal" className="min-h-0 flex-1 overflow-hidden">
        <ResizablePanel
          panelRef={sidebarRef}
          defaultSize="16%"
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

        <ResizablePanel defaultSize="84%" minSize="45%" className="overflow-hidden">
          <IdeEditorAndResults orgSlug={orgSlug} workspace={workspace} />
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  )
}

// ─── Sidebar ───────────────────────────────────────────────────────────────────

function IdeSidebar({ orgSlug, workspace }: { orgSlug: string; workspace: Workspace }) {
  const activeActivityId = useIde((s) => s.activeActivityId)
  const runtimeSettings = useQuery(orgRuntimeSettingsQueryOptions(orgSlug))
  const visibilityContext: ActivityVisibilityContext = {
    queryHistoryMode: runtimeSettings.data?.effective.query_history_mode ?? 'backend',
    queryFavoritesMode: runtimeSettings.data?.effective.query_favorites_mode ?? 'backend',
  }
  const activities = visibleActivities(visibilityContext)
  const active =
    activities.find((a) => a.id === activeActivityId && a.mode === 'sidebar') ??
    activities.find((a) => a.mode === 'sidebar')

  if (!active) return null
  const Panel = active.component

  return (
    <aside className="flex h-full min-h-0 flex-col border-r border-border bg-sidebar">
      <Panel orgSlug={orgSlug} workspace={workspace} />
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
    <ResizablePanelGroup orientation="vertical" className="min-h-0 flex-1 overflow-hidden">
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
        <ResultsArea orgSlug={orgSlug} workspace={workspace} />
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

  const activeTabId = useIde((s) => selectActiveTabId(s, workspace.id))
  const layout = useIde((s) => s.layout[workspace.id])
  const draggingTab = useIde((s) => s.draggingTab)
  const splitTabToEdge = useIde((s) => s.splitTabToEdge)
  const tabs = useIde((s) => s.tabs)
  const openConsole = useIde((s) => s.openConsole)
  const openTab = useIde((s) => s.openTab)
  const updateTabContent = useIde((s) => s.updateTabContent)
  const saveEditorTab = useSaveEditorTab(orgSlug, workspace.id)

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

  useEditorDocumentLifecycle(tabs, registry)
  useClosedFileCacheCleanup(tabs, orgSlug, queryClient)
  useEditorSnapshotPersistence(tabs, registry, updateTabContent)

  // Reset cursor info when the active tab changes.
  useEffect(() => {
    setCursorInfo(null)
  }, [activeTabId])

  // No "ensure one tab" guard — users can close all tabs to reach the empty state.

  useEditorSaveShortcut(activeTab, saveEditorTab)

  // ── Render ─────────────────────────────────────────────────────────────────
  // Each EditorGroup populates its own Y.Doc synchronously and loads its own file
  // content; EditorSection keeps the workspace-wide Y.Doc lifecycle effects above.
  const hasAnyTab = tabs.some((t) => t.workspaceId === workspace.id)

  return (
    <>
      <section className="flex h-full min-h-0 flex-col bg-background">
        <IdeToolbar orgSlug={orgSlug} workspace={workspace} />
        <div className="relative min-h-0 flex-1">
          {hasAnyTab && layout ? (
            <EditorLayout
              orgSlug={orgSlug}
              workspace={workspace}
              node={layout}
              onCursorChange={(line, col, sel) => setCursorInfo({ line, col, sel })}
            />
          ) : (
            <div className="h-full border-t border-border bg-card">
              <EmptyEditorState
                onNewConsole={createConsole}
                onNewFile={() => setCreateFileOpen(true)}
              />
            </div>
          )}
          {/* Drag a tab to the left/right edge to open it in a new split. */}
          {draggingTab && hasAnyTab && (
            <>
              <EdgeDropZone
                side="left"
                onDropTab={() => splitTabToEdge(workspace.id, draggingTab.tabId, 'left')}
              />
              <EdgeDropZone
                side="right"
                onDropTab={() => splitTabToEdge(workspace.id, draggingTab.tabId, 'right')}
              />
            </>
          )}
        </div>
        <EditorStatusBar cursorInfo={cursorInfo} hasActiveTab={!!activeTab} />
      </section>

      <SaveAsDialog
        open={createFileOpen}
        onOpenChange={(open) => {
          if (!open) setCreateFileOpen(false)
        }}
        tab={{
          id: 'new-empty',
          workspaceId: workspace.id,
          title: 'untitled',
          kind: 'scratch',
          content: '',
        }}
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

function EditorStatusBar({
  cursorInfo,
  hasActiveTab,
}: {
  cursorInfo: CursorInfo | null
  hasActiveTab: boolean
}) {
  const maximizedPane = useIde((s) => s.maximizedPane)
  const setMaximizedPane = useIde((s) => s.setMaximizedPane)
  const resultsVisible = maximizedPane !== 'editor'

  return (
    <div className="flex h-6 shrink-0 items-center gap-3 bg-sidebar pl-3 pr-1 text-[11px] text-muted-foreground">
      {hasActiveTab && cursorInfo && (
        <>
          <span className="tabular-nums">
            Ln {cursorInfo.line}, Col {cursorInfo.col}
          </span>
          {cursorInfo.sel > 0 && <span className="tabular-nums">{cursorInfo.sel} selected</span>}
        </>
      )}
      <div className="flex-1" />
      {hasActiveTab && <span className="font-medium">SQL</span>}
      <Tip label={resultsVisible ? 'Hide results panel' : 'Show results panel'}>
        <button
          type="button"
          aria-label={resultsVisible ? 'Hide results panel' : 'Show results panel'}
          aria-pressed={resultsVisible}
          onClick={() => setMaximizedPane(resultsVisible ? 'editor' : null)}
          className={cn(
            'flex h-5 items-center rounded-sm px-1.5 transition-colors hover:bg-sidebar-accent hover:text-foreground',
            resultsVisible ? 'text-foreground' : 'text-muted-foreground',
          )}
        >
          <Icon name="layout-bottom" size={12} />
        </button>
      </Tip>
    </div>
  )
}

// ─── Edge drop zone (drag a tab here to split) ──────────────────────────────────

function EdgeDropZone({ side, onDropTab }: { side: 'left' | 'right'; onDropTab: () => void }) {
  const [over, setOver] = useState(false)
  return (
    <div
      onDragOver={(e) => {
        e.preventDefault()
        e.dataTransfer.dropEffect = 'move'
        setOver(true)
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        e.preventDefault()
        setOver(false)
        onDropTab()
      }}
      // A directional fade (solid at the edge, fading into the editor) signals
      // "drop here to split" without implying the new pane will be this width.
      // Starts below the tab-bar row (h-9) so tabs stay draggable.
      style={{
        background: `linear-gradient(${side === 'left' ? 'to right' : 'to left'}, color-mix(in oklab, var(--primary) 32%, transparent), transparent)`,
      }}
      className={cn(
        'absolute bottom-0 top-9 z-20 w-[16%] transition-opacity duration-150',
        side === 'left' ? 'left-0' : 'right-0',
        over ? 'opacity-100' : 'opacity-0',
      )}
    />
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
      <div className="flex flex-col items-center gap-3 text-center">
        <div className="flex size-11 items-center justify-center rounded-xl border border-border bg-muted/50">
          <Icon name="terminal" size={20} className="text-muted-foreground" />
        </div>
        <div>
          <p className="text-sm font-semibold text-foreground">No editors open</p>
          <p className="mt-1 text-xs text-muted-foreground">
            Start by opening a console or a file.
          </p>
        </div>
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
        'group flex w-48 flex-col items-center gap-3 rounded-xl border border-border bg-background/60 p-5 text-center',
        'transition-all hover:border-primary/40 hover:bg-accent/50 hover:shadow-sm',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
      )}
    >
      <div
        className={cn(
          'flex size-10 items-center justify-center rounded-lg',
          'bg-muted transition-colors group-hover:bg-primary/10',
        )}
      >
        <Icon
          name={icon}
          size={18}
          className="text-muted-foreground transition-colors group-hover:text-primary"
        />
      </div>
      <div className="flex flex-col gap-1">
        <p className="text-xs font-medium text-foreground">{title}</p>
        <p className="text-[11px] leading-relaxed text-muted-foreground">{description}</p>
      </div>
    </button>
  )
}

// ─── Utility ───────────────────────────────────────────────────────────────────
