import { createContext, useContext } from 'react'
import { createStore, useStore } from 'zustand'
import { persist, createJSONStorage } from 'zustand/middleware'
import type { Connection, ObjectRef, ResultSet, Workspace, WorkspaceFile } from '#/lib/api/types'
import { makeRoleGatedStorage, electPrimary, type WindowRole } from './windowRole'
import {
  createGroup,
  addTab,
  removeTab,
  removeTabFromGroup,
  setActive,
  setActiveInGroup,
  moveTabBetweenGroups,
  splitGroup,
  splitToEdge,
  placeTabAtEdge,
  findGroup,
  firstGroup,
  allGroups,
  migrateToLayout,
  type LayoutNode,
  type SplitDirection,
} from './ideLayout'

// ─── Types ─────────────────────────────────────────────────────────────────────

export type QueryResult =
  | { status: 'idle' }
  | { status: 'running' }
  | { status: 'ok'; data: ResultSet; durationMs: number; sql: string; isFetchingNextPage?: boolean; cursorMessage?: string }
  | { status: 'error'; message: string; sql: string }
  | { status: 'cancelled'; sql: string }

export type TabKind = 'scratch' | 'file' | 'connection' | 'object'

export type EditorTab = {
  id: string
  workspaceId: number
  title: string
  kind: TabKind
  subtitle?: string
  connectionId?: number
  driver?: string
  fileId?: number
  /** Set for `object` tabs: the qualified database object this tab views. */
  objectRef?: ObjectRef
  etag?: string
  isDirty?: boolean
  content: string
  /** Canonical Y.js initial state (number[] so it survives JSON persist).
   *  Set once at creation time by the opening window; applied by all windows
   *  to guarantee identical Y.js histories and correct incremental sync. */
  yState?: number[]
  /** Current Y.js state snapshot, updated alongside tab.content on each
   *  debounced write. On page reload this is used to restore the Y.Doc so
   *  unsaved console edits and dirty file edits survive a browser refresh. */
  ySnapshot?: number[]
}

export type IdeState = {
  activeWorkspaceId?: number
  maximizedPane: 'editor' | 'results' | null
  /** Active sidebar/page activity id (see ideActivities). */
  activeActivityId: string
  sidebarCollapsed: boolean
  /** Per-workspace editor layout tree (split groups + their tab lists). */
  layout: Record<number, LayoutNode>
  /** Focused group id per workspace — drives which group Run/Save/results target. */
  activeGroupId: Record<number, string>
  /** Tab + source group currently being dragged (transient; drives edge-split drop zones). */
  draggingTab: { tabId: string; fromGroupId: string } | null
  /** Transient per-connection connect status (not persisted): 'connecting' or an error. */
  connectionStatus: Record<number, 'connecting' | { error: string }>
  /** Persisted expansion of explorer nodes, keyed `env:<id>` / `conn:<id>` / `folder:<id>`. */
  expandedNodes: Record<string, boolean>
  /** `${groupId}:${tabId}` of an editor that should grab keyboard focus once mounted (after a split). */
  focusEditorRequest: string | null
  tabs: EditorTab[]
  /** Live session IDs keyed by connectionId. A session entry means the backend has an open pool connection for this account. */
  sessions: Record<number, string>
  /** Last query result per tab ID. Not persisted to IndexedDB. */
  results: Record<string, QueryResult>
  /** Tabs with an in-flight query. Not persisted to IndexedDB. */
  runningTabs: Record<string, boolean>
  /** AbortControllers for in-flight fetch requests, keyed by tabId. Not persisted. */
  abortControllers: Record<string, AbortController>
}

export type IdeActions = {
  setActiveWorkspace: (workspaceId: number) => void
  openTab: (tab: EditorTab) => void
  openTabToSide: (tab: EditorTab) => void
  ensureTab: (tab: EditorTab) => void
  closeTab: (tabId: string) => void
  /** Close one pane's instance of a tab; releases the tab only when no group still holds it. */
  closeTabInstance: (groupId: string, tabId: string) => void
  /** Activate a tab within a specific group and focus that group. */
  setActiveTab: (groupId: string, tabId: string) => void
  /** Move a tab instance from one group to another (or reorder within a group) by drag. */
  moveTab: (
    fromGroupId: string,
    draggedTabId: string,
    toGroupId: string,
    targetTabId: string,
    position: 'before' | 'after',
  ) => void
  /** Focus a group (drives Run/Save/results target). */
  focusGroup: (workspaceId: number, groupId: string) => void
  /** Duplicate a group's tab into a new group beside it in the given direction. */
  splitActiveTab: (workspaceId: number, groupId: string, tabId: string, direction: SplitDirection) => void
  /** Duplicate a tab into a new group at the left/right editor edge (edge drop). */
  splitTabToEdge: (workspaceId: number, tabId: string, side: 'left' | 'right') => void
  /** Persist a split node's child sizes after a resize. */
  setSplitSizes: (workspaceId: number, splitId: string, sizes: number[]) => void
  /** Track the tab + source group being dragged (transient drag UI state). */
  setDraggingTab: (drag: { tabId: string; fromGroupId: string } | null) => void
  /** Set (or clear, with null) a connection's transient connect status. */
  setConnectionStatus: (connectionId: number, status: 'connecting' | { error: string } | null) => void
  /** Persist a node's expanded state. */
  setNodeExpanded: (key: string, expanded: boolean) => void
  /** Collapse every remembered explorer node. */
  collapseAllNodes: () => void
  /** Request (or clear) keyboard focus for a specific editor pane. */
  setFocusEditorRequest: (key: string | null) => void
  updateTabContent: (tabId: string, content: string, ySnapshot?: number[]) => void
  updateTabEtag: (tabId: string, etag: string) => void
  setTabConnection: (tabId: string, connectionId: number, driver?: string) => void
  setMaximizedPane: (pane: IdeState['maximizedPane']) => void
  setActiveActivity: (activityId: string) => void
  setSidebarCollapsed: (collapsed: boolean) => void
  setSession: (connectionId: number, sessionId: string) => void
  clearSession: (connectionId: number) => void
  /** Replace the entire sessions map with authoritative data from the backend. */
  syncSessions: (backendSessions: Record<number, string>) => void
  setQueryResult: (tabId: string, result: QueryResult) => void
  setTabRunning: (tabId: string, running: boolean) => void
  setTabController: (tabId: string, controller: AbortController | null) => void
  /** Opens a new numbered console tab. Pass yState (encoded Y.Doc) so all windows
   *  that receive this tab share the same canonical Y.js initial history.
   *  Pass connectionId to pre-select a connection on the new tab. */
  openConsole: (workspace: Workspace, yState: number[], connectionId?: number) => void
}

// ─── Layout selectors ───────────────────────────────────────────────────────────

let _groupSeq = 0
const newGroupId = () => `grp-${Date.now().toString(36)}-${(_groupSeq++).toString(36)}`

/** Returns a workspace's layout + focused group id, creating an empty group if none exists. */
function ensureWorkspaceLayout(s: IdeState, workspaceId: number): { layout: LayoutNode; groupId: string } {
  const existing = s.layout[workspaceId]
  if (existing) {
    const focusedId = s.activeGroupId[workspaceId]
    const group = (focusedId ? findGroup(existing, focusedId) : undefined) ?? firstGroup(existing)
    return { layout: existing, groupId: group.id }
  }
  const group = createGroup(newGroupId(), [])
  return { layout: group, groupId: group.id }
}

/** The focused group's active tab id for a workspace (replaces the old activeTabIds[ws]). */
export function activeTabId(s: IdeState, workspaceId: number): string | undefined {
  const layout = s.layout[workspaceId]
  if (!layout) return undefined
  const focusedId = s.activeGroupId[workspaceId]
  const group = (focusedId ? findGroup(layout, focusedId) : undefined) ?? firstGroup(layout)
  return group?.activeTabId
}

export type ConnectionState =
  | { kind: 'connected' }
  | { kind: 'connecting' }
  | { kind: 'error'; message: string }
  | { kind: 'idle' }

/** A node's expanded state: the stored value, or `fallback` when there's no record. */
export function isNodeExpanded(nodes: Record<string, boolean>, key: string, fallback: boolean): boolean {
  return nodes[key] ?? fallback
}

/** Resolves a connection's display state from its (stable) inputs. Kept separate
 *  from a store selector so callers can select primitive/stable values and avoid
 *  re-render loops from returning a fresh object on every render. */
export function resolveConnectionState(
  hasSession: boolean,
  status: 'connecting' | { error: string } | undefined,
): ConnectionState {
  if (hasSession) return { kind: 'connected' }
  if (status === 'connecting') return { kind: 'connecting' }
  if (status && typeof status === 'object') return { kind: 'error', message: status.error }
  return { kind: 'idle' }
}

/** Resolves a connection's display state. A live session means connected,
 *  regardless of any prior error; otherwise the transient status decides. */
export function connectionState(s: IdeState, connectionId: number): ConnectionState {
  return resolveConnectionState(Boolean(s.sessions[connectionId]), s.connectionStatus[connectionId])
}

// ─── Store factory ─────────────────────────────────────────────────────────────

export function createIdeStore(orgSlug: string, accountId: number, role: WindowRole = 'managed') {
  const storageKey = `sqlwarden.ide.${orgSlug}.${accountId}`
  // Ephemeral windows never persist; managed windows persist only while they hold
  // the primary lock (one window at a time).
  let isPrimary = false
  const canPersist = () => role === 'managed' && isPrimary

  const store = createStore<IdeState & IdeActions>()(
    persist(
      (set) => ({
        activeWorkspaceId: undefined,
        maximizedPane: null,
        activeActivityId: 'connections',
        sidebarCollapsed: false,
        layout: {},
        activeGroupId: {},
        draggingTab: null,
        focusEditorRequest: null,
        connectionStatus: {},
        expandedNodes: {},
        tabs: [],
        sessions: {},
        results: {},
        runningTabs: {},
        abortControllers: {},

        setActiveWorkspace: (id) => set({ activeWorkspaceId: id }),

        openTab: (tab) =>
          set((s) => {
            const exists = s.tabs.some((t) => t.id === tab.id)
            const { layout, groupId } = ensureWorkspaceLayout(s, tab.workspaceId)
            const nextLayout = setActive(addTab(layout, groupId, tab.id), tab.id)
            return {
              tabs: exists ? s.tabs : [...s.tabs, tab],
              layout: { ...s.layout, [tab.workspaceId]: nextLayout },
              activeGroupId: { ...s.activeGroupId, [tab.workspaceId]: groupId },
            }
          }),

        openTabToSide: (tab) =>
          set((s) => {
            const exists = s.tabs.some((t) => t.id === tab.id)
            const { layout, groupId } = ensureWorkspaceLayout(s, tab.workspaceId)
            const nextTabs = exists ? s.tabs : [...s.tabs, tab]
            // A fresh/empty workspace has nothing to sit beside — open in place.
            if (!allGroups(layout).some((g) => g.tabIds.length > 0)) {
              return {
                tabs: nextTabs,
                layout: { ...s.layout, [tab.workspaceId]: setActive(addTab(layout, groupId, tab.id), tab.id) },
                activeGroupId: { ...s.activeGroupId, [tab.workspaceId]: groupId },
              }
            }
            const { node, newGroupId: gid } = placeTabAtEdge(layout, tab.id, 'right', newGroupId())
            return {
              tabs: nextTabs,
              layout: { ...s.layout, [tab.workspaceId]: node },
              activeGroupId: { ...s.activeGroupId, [tab.workspaceId]: gid },
              focusEditorRequest: `${gid}:${tab.id}`,
            }
          }),

        ensureTab: (tab) =>
          set((s) => {
            if (s.tabs.some((t) => t.id === tab.id)) return {}
            const { layout, groupId } = ensureWorkspaceLayout(s, tab.workspaceId)
            return {
              tabs: [...s.tabs, tab],
              layout: { ...s.layout, [tab.workspaceId]: addTab(layout, groupId, tab.id) },
              activeGroupId: { ...s.activeGroupId, [tab.workspaceId]: s.activeGroupId[tab.workspaceId] ?? groupId },
            }
          }),

        closeTab: (tabId) =>
          set((s) => {
            s.abortControllers[tabId]?.abort()
            const closedTab = s.tabs.find((t) => t.id === tabId)
            const nextTabs = s.tabs.filter((t) => t.id !== tabId)
            const { [tabId]: _r, ...nextResults } = s.results
            const { [tabId]: _rt, ...nextRunning } = s.runningTabs
            const { [tabId]: _ac, ...nextControllers } = s.abortControllers

            const patch = {
              tabs: nextTabs,
              results: nextResults,
              runningTabs: nextRunning,
              abortControllers: nextControllers,
            }
            if (!closedTab) return patch

            const ws = closedTab.workspaceId
            const layout = s.layout[ws]
            if (!layout) return patch
            const nextLayout = removeTab(layout, tabId)
            const focused = s.activeGroupId[ws]
            const stillThere = focused && findGroup(nextLayout, focused)
            return {
              ...patch,
              layout: { ...s.layout, [ws]: nextLayout },
              activeGroupId: { ...s.activeGroupId, [ws]: stillThere ? focused : firstGroup(nextLayout).id },
            }
          }),

        closeTabInstance: (groupId, tabId) =>
          set((s) => {
            const tab = s.tabs.find((t) => t.id === tabId)
            if (!tab) return {}
            const ws = tab.workspaceId
            const layout = s.layout[ws]
            if (!layout) return {}
            const nextLayout = removeTabFromGroup(layout, groupId, tabId)
            const focused = s.activeGroupId[ws]
            const focusStillThere = focused && findGroup(nextLayout, focused)
            const base = {
              layout: { ...s.layout, [ws]: nextLayout },
              activeGroupId: { ...s.activeGroupId, [ws]: focusStillThere ? focused : firstGroup(nextLayout).id },
            }
            // Keep the tab and its Y.Doc alive while another pane still shows it.
            if (allGroups(nextLayout).some((g) => g.tabIds.includes(tabId))) return base
            s.abortControllers[tabId]?.abort()
            const { [tabId]: _r, ...nextResults } = s.results
            const { [tabId]: _rt, ...nextRunning } = s.runningTabs
            const { [tabId]: _ac, ...nextControllers } = s.abortControllers
            return {
              ...base,
              tabs: s.tabs.filter((t) => t.id !== tabId),
              results: nextResults,
              runningTabs: nextRunning,
              abortControllers: nextControllers,
            }
          }),

        setActiveTab: (groupId, tabId) =>
          set((s) => {
            const tab = s.tabs.find((t) => t.id === tabId)
            if (!tab) return {}
            const ws = tab.workspaceId
            const layout = s.layout[ws]
            if (!layout || !findGroup(layout, groupId)) return {}
            return {
              layout: { ...s.layout, [ws]: setActiveInGroup(layout, groupId, tabId) },
              activeGroupId: { ...s.activeGroupId, [ws]: groupId },
            }
          }),

        moveTab: (fromGroupId, draggedTabId, toGroupId, targetTabId, position) =>
          set((s) => {
            const tab = s.tabs.find((t) => t.id === draggedTabId)
            if (!tab) return {}
            const ws = tab.workspaceId
            const layout = s.layout[ws]
            if (!layout) return {}
            const next = moveTabBetweenGroups(layout, fromGroupId, draggedTabId, toGroupId, targetTabId, position)
            if (next === layout) return {}
            // The dragged tab is now active in the target group; focus it if it survived.
            const focusId = findGroup(next, toGroupId) ? toGroupId : firstGroup(next).id
            return {
              layout: { ...s.layout, [ws]: next },
              activeGroupId: { ...s.activeGroupId, [ws]: focusId },
            }
          }),

        focusGroup: (workspaceId, groupId) =>
          set((s) => ({ activeGroupId: { ...s.activeGroupId, [workspaceId]: groupId } })),

        splitActiveTab: (workspaceId, groupId, tabId, direction) =>
          set((s) => {
            const layout = s.layout[workspaceId]
            if (!layout) return {}
            const { node, newGroupId: gid } = splitGroup(layout, groupId, tabId, direction, newGroupId())
            if (node === layout) return {}
            // Focus the newly created pane (the tab now lives in both groups) and
            // give its editor keyboard focus once it mounts.
            return {
              layout: { ...s.layout, [workspaceId]: node },
              activeGroupId: { ...s.activeGroupId, [workspaceId]: gid },
              focusEditorRequest: `${gid}:${tabId}`,
            }
          }),

        splitTabToEdge: (workspaceId, tabId, side) =>
          set((s) => {
            const layout = s.layout[workspaceId]
            if (!layout) return {}
            const { node, newGroupId: gid } = splitToEdge(layout, tabId, side, newGroupId())
            if (node === layout) return {}
            return {
              layout: { ...s.layout, [workspaceId]: node },
              activeGroupId: { ...s.activeGroupId, [workspaceId]: gid },
              focusEditorRequest: `${gid}:${tabId}`,
            }
          }),

        setSplitSizes: (workspaceId, splitId, sizes) =>
          set((s) => {
            const layout = s.layout[workspaceId]
            if (!layout) return {}
            const apply = (n: LayoutNode): LayoutNode =>
              n.type === 'group'
                ? n
                : { ...n, sizes: n.id === splitId ? sizes : n.sizes, children: n.children.map(apply) }
            return { layout: { ...s.layout, [workspaceId]: apply(layout) } }
          }),

        setDraggingTab: (drag) => set({ draggingTab: drag }),

        setConnectionStatus: (connectionId, status) =>
          set((s) => {
            if (status === null) {
              const { [connectionId]: _drop, ...rest } = s.connectionStatus
              return { connectionStatus: rest }
            }
            return { connectionStatus: { ...s.connectionStatus, [connectionId]: status } }
          }),

        setNodeExpanded: (key, expanded) =>
          set((s) => ({ expandedNodes: { ...s.expandedNodes, [key]: expanded } })),

        collapseAllNodes: () => set({ expandedNodes: {} }),

        setFocusEditorRequest: (key) => set({ focusEditorRequest: key }),

        updateTabContent: (tabId, content, ySnapshot?) =>
          set((s) => ({
            tabs: s.tabs.map((t) =>
              t.id === tabId
                ? {
                  ...t,
                  content,
                  ...(ySnapshot !== undefined ? { ySnapshot } : {}),
                  isDirty: t.etag !== undefined && content !== t.content ? true : t.isDirty,
                }
                : t,
            ),
          })),

        updateTabEtag: (tabId, etag) =>
          set((s) => ({
            tabs: s.tabs.map((t) => (t.id === tabId ? { ...t, etag, isDirty: false } : t)),
          })),

        setTabConnection: (tabId, connectionId, driver?) =>
          set((s) => ({ tabs: s.tabs.map((t) => (t.id === tabId ? { ...t, connectionId, ...(driver ? { driver } : {}) } : t)) })),

        setMaximizedPane: (pane) => set({ maximizedPane: pane }),
        setActiveActivity: (activityId) => set({ activeActivityId: activityId }),
        setSidebarCollapsed: (collapsed) => set({ sidebarCollapsed: collapsed }),

        setSession: (connectionId, sessionId) =>
          set((s) => ({ sessions: { ...s.sessions, [connectionId]: sessionId } })),

        clearSession: (connectionId) =>
          set((s) => {
            const { [connectionId]: _removed, ...rest } = s.sessions
            return { sessions: rest }
          }),

        syncSessions: (backendSessions) => set({ sessions: backendSessions }),

        setQueryResult: (tabId, result) =>
          set((s) => ({ results: { ...s.results, [tabId]: result } })),

        setTabRunning: (tabId, running) =>
          set((s) => ({ runningTabs: { ...s.runningTabs, [tabId]: running } })),

        setTabController: (tabId, controller) =>
          set((s) => {
            if (controller === null) {
              const { [tabId]: _ac, ...rest } = s.abortControllers
              return { abortControllers: rest }
            }
            return { abortControllers: { ...s.abortControllers, [tabId]: controller } }
          }),

        openConsole: (workspace, yState, connectionId) =>
          set((s) => {
            const wsConsoleTabs = s.tabs.filter(
              (t) => t.workspaceId === workspace.id && t.kind === 'scratch',
            )
            const maxNum = wsConsoleTabs.reduce((max, t) => {
              const m = t.id.match(/^scratch:\d+:(\d+)$/)
              return m ? Math.max(max, parseInt(m[1], 10)) : max
            }, 0)
            const num = maxNum + 1
            const tab: EditorTab = {
              id: `scratch:${workspace.id}:${num}`,
              workspaceId: workspace.id,
              title: `Console ${num}`,
              kind: 'scratch',
              content: DEFAULT_CONSOLE_CONTENT,
              yState,
              ...(connectionId !== undefined ? { connectionId } : {}),
            }
            const exists = s.tabs.some((t) => t.id === tab.id)
            const { layout, groupId } = ensureWorkspaceLayout(s, workspace.id)
            return {
              tabs: exists ? s.tabs : [...s.tabs, tab],
              layout: { ...s.layout, [workspace.id]: setActive(addTab(layout, groupId, tab.id), tab.id) },
              activeGroupId: { ...s.activeGroupId, [workspace.id]: groupId },
            }
          }),
      }),
      {
        name: storageKey,
        version: 1,
        // Migrate v0 state (flat `activeTabIds`) to the v1 layout tree: one group
        // per workspace built from its tabs + the old active tab. Zero data loss.
        migrate: (persisted, version) => {
          const state = persisted as (IdeState & { activeTabIds?: Record<number, string> }) | undefined
          if (!state) return persisted as IdeState
          if (version >= 1 && state.layout) return state as IdeState
          const byWs: Record<number, string[]> = {}
          for (const tab of state.tabs ?? []) (byWs[tab.workspaceId] ??= []).push(tab.id)
          const layout: Record<number, LayoutNode> = {}
          const activeGroupId: Record<number, string> = {}
          for (const [wsStr, ids] of Object.entries(byWs)) {
            const ws = Number(wsStr)
            const gid = `grp-mig-${ws}`
            layout[ws] = migrateToLayout(gid, ids, state.activeTabIds?.[ws])
            activeGroupId[ws] = gid
          }
          const { activeTabIds: _drop, ...rest } = state
          return { ...rest, layout, activeGroupId } as IdeState
        },
        storage: createJSONStorage(() =>
          role === 'ephemeral'
            ? { getItem: async () => null, setItem: async () => {}, removeItem: async () => {} }
            : makeRoleGatedStorage(storageKey, canPersist),
        ),
        // Exclude ephemeral query results from IndexedDB — they can be large
        // and are meaningless after a page reload anyway.
        partialize: ({ results: _r, runningTabs: _rt, abortControllers: _ac, draggingTab: _dt, focusEditorRequest: _fe, connectionStatus: _cs, ...state }) => state,
      },
    ),
  )

  // Compete for the primary lock; the holder is the only window that persists.
  const cleanupElection =
    role === 'ephemeral'
      ? () => {}
      : electPrimary(`sqlwarden-ide-primary:${orgSlug}:${accountId}`, () => {
          isPrimary = true
        })
  ;(store as unknown as { __cleanupElection?: () => void }).__cleanupElection = cleanupElection

  return store
}

// ─── React context + hook ──────────────────────────────────────────────────────

export type IdeStore = ReturnType<typeof createIdeStore>

export const IdeStoreContext = createContext<IdeStore | null>(null)

// No-op fallback used when useIde is called without a provider. This happens
// during React's dev-mode error-recovery replay render, which invokes render
// functions against the committed tree (where the provider may not exist yet)
// to generate an accurate stack trace. Using a fallback means the replay
// renders harmlessly with empty state instead of crashing, allowing the real
// error boundary to show a useful UI.
const _noop = () => {}
const _contextFallback = createStore<IdeState & IdeActions>()(() => ({
  activeWorkspaceId: undefined,
  maximizedPane: null,
  activeActivityId: 'connections',
  sidebarCollapsed: false,
  layout: {},
  activeGroupId: {},
  draggingTab: null,
  focusEditorRequest: null,
  connectionStatus: {},
  expandedNodes: {},
  tabs: [],
  sessions: {},
  results: {},
  runningTabs: {},
  abortControllers: {},
  setActiveWorkspace: _noop,
  openTab: _noop,
  openTabToSide: _noop,
  ensureTab: _noop,
  closeTab: _noop,
  closeTabInstance: _noop,
  setActiveTab: _noop,
  moveTab: _noop,
  focusGroup: _noop,
  splitActiveTab: _noop,
  splitTabToEdge: _noop,
  setSplitSizes: _noop,
  setDraggingTab: _noop,
  setFocusEditorRequest: _noop,
  setConnectionStatus: _noop,
  setNodeExpanded: _noop,
  collapseAllNodes: _noop,
  updateTabContent: _noop,
  updateTabEtag: _noop,
  setTabConnection: _noop,
  setMaximizedPane: _noop,
  setActiveActivity: _noop,
  setSidebarCollapsed: _noop,
  setSession: _noop,
  clearSession: _noop,
  syncSessions: _noop,
  setQueryResult: _noop,
  setTabRunning: _noop,
  setTabController: _noop,
  openConsole: _noop,
}))

export function useIde<T>(selector: (state: IdeState & IdeActions) => T): T {
  const store = useContext(IdeStoreContext) ?? _contextFallback
  return useStore(store, selector)
}

// ─── Tab factory functions ──────────────────────────────────────────────────────

export const DEFAULT_CONSOLE_CONTENT = '';

/** @deprecated Use the openConsole store action instead, which gives consoles
 *  stable numbered IDs and embeds a canonical Y.js initial state for cross-window sync. */
export function newScratchTab(workspace: Workspace): EditorTab {
  const ts = Date.now()
  return {
    id: `scratch:${workspace.id}:${ts}`,
    workspaceId: workspace.id,
    title: `Console ${new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`,
    kind: 'scratch',
    content: DEFAULT_CONSOLE_CONTENT,
  }
}

export function newConnectionTab(connection: Connection, workspace: Workspace): EditorTab {
  return {
    id: `connection:${connection.id}`,
    workspaceId: workspace.id,
    title: connection.name,
    kind: 'connection',
    subtitle: connection.driver,
    connectionId: connection.id,
    driver: connection.driver,
    content: `-- ${connection.name}\nSELECT 1;`,
  }
}

export function newFileTab(file: WorkspaceFile, workspace: Workspace): EditorTab {
  return {
    id: `file:${file.id}`,
    workspaceId: workspace.id,
    title: file.name,
    kind: 'file',
    subtitle: file.name,
    fileId: file.id,
    content: '',
  }
}
