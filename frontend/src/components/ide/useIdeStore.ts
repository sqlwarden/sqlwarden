import { createContext, useContext } from 'react'
import { createStore, useStore } from 'zustand'
import { persist, createJSONStorage, type StateStorage } from 'zustand/middleware'
import { get, set, del } from 'idb-keyval'
import type { Connection, ResultSet, Workspace, WorkspaceFile } from '#/lib/api/types'

// ─── Types ─────────────────────────────────────────────────────────────────────

export type QueryResult =
  | { status: 'idle' }
  | { status: 'running' }
  | { status: 'ok'; data: ResultSet; durationMs: number; sql: string }
  | { status: 'error'; message: string; sql: string }
  | { status: 'cancelled'; sql: string }

export type TabKind = 'scratch' | 'file' | 'connection'

export type EditorTab = {
  id: string
  workspaceId: number
  title: string
  kind: TabKind
  subtitle?: string
  connectionId?: number
  driver?: string
  fileId?: number
  etag?: string
  isDirty?: boolean
  content: string
  /** Canonical Y.js initial state (number[] so it survives JSON persist).
   *  Set once at creation time by the opening window; applied by all windows
   *  to guarantee identical Y.js histories and correct incremental sync. */
  yState?: number[]
}

export type IdeState = {
  activeWorkspaceId?: number
  maximizedPane: 'editor' | 'results' | null
  maximizedSidebarPane: 'database' | 'files' | null
  sidebarCollapsed: boolean
  activeTabId?: string
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
  ensureTab: (tab: EditorTab) => void
  closeTab: (tabId: string) => void
  setActiveTab: (tabId: string) => void
  updateTabContent: (tabId: string, content: string) => void
  updateTabEtag: (tabId: string, etag: string) => void
  setTabConnection: (tabId: string, connectionId: number, driver?: string) => void
  setMaximizedPane: (pane: IdeState['maximizedPane']) => void
  setMaximizedSidebarPane: (pane: IdeState['maximizedSidebarPane']) => void
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

// ─── Store factory ─────────────────────────────────────────────────────────────

function makeStorage(orgSlug: string, accountId: number): StateStorage {
  const key = `sqlwarden.ide.${orgSlug}.${accountId}`
  return {
    getItem: async () => {
      const val = await get<string>(key)
      return val ?? null
    },
    setItem: async (_name, value) => set(key, value),
    removeItem: async () => del(key),
  }
}

export function createIdeStore(orgSlug: string, accountId: number) {
  return createStore<IdeState & IdeActions>()(
    persist(
      (set) => ({
        activeWorkspaceId: undefined,
        maximizedPane: null,
        maximizedSidebarPane: null,
        sidebarCollapsed: false,
        activeTabId: undefined,
        tabs: [],
        sessions: {},
        results: {},
        runningTabs: {},
        abortControllers: {},

        setActiveWorkspace: (id) => set({ activeWorkspaceId: id }),

        openTab: (tab) =>
          set((s) => {
            const existing = s.tabs.find((t) => t.id === tab.id)
            return { activeTabId: tab.id, tabs: existing ? s.tabs : [...s.tabs, tab] }
          }),

        ensureTab: (tab) =>
          set((s) => {
            if (s.tabs.some((t) => t.id === tab.id)) return s
            return { tabs: [...s.tabs, tab] }
          }),

        closeTab: (tabId) =>
          set((s) => {
            s.abortControllers[tabId]?.abort()
            const nextTabs = s.tabs.filter((t) => t.id !== tabId)
            const nextActive =
              s.activeTabId === tabId
                ? nextTabs.find((t) => t.workspaceId === s.activeWorkspaceId)?.id
                : s.activeTabId
            const { [tabId]: _r, ...nextResults } = s.results
            const { [tabId]: _rt, ...nextRunning } = s.runningTabs
            const { [tabId]: _ac, ...nextControllers } = s.abortControllers
            return { tabs: nextTabs, activeTabId: nextActive, results: nextResults, runningTabs: nextRunning, abortControllers: nextControllers }
          }),

        setActiveTab: (id) => set({ activeTabId: id }),

        updateTabContent: (tabId, content) =>
          set((s) => ({
            tabs: s.tabs.map((t) =>
              t.id === tabId
                ? {
                  ...t,
                  content,
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
        setMaximizedSidebarPane: (pane) => set({ maximizedSidebarPane: pane }),
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
            return {
              activeTabId: tab.id,
              tabs: exists ? s.tabs : [...s.tabs, tab],
            }
          }),
      }),
      {
        name: `sqlwarden.ide.${orgSlug}.${accountId}`,
        storage: createJSONStorage(() => makeStorage(orgSlug, accountId)),
        // Exclude ephemeral query results from IndexedDB — they can be large
        // and are meaningless after a page reload anyway.
        partialize: ({ results: _r, runningTabs: _rt, abortControllers: _ac, ...state }) => state,
      },
    ),
  )
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
  maximizedSidebarPane: null,
  sidebarCollapsed: false,
  activeTabId: undefined,
  tabs: [],
  sessions: {},
  results: {},
  runningTabs: {},
  abortControllers: {},
  setActiveWorkspace: _noop,
  openTab: _noop,
  ensureTab: _noop,
  closeTab: _noop,
  setActiveTab: _noop,
  updateTabContent: _noop,
  updateTabEtag: _noop,
  setTabConnection: _noop,
  setMaximizedPane: _noop,
  setMaximizedSidebarPane: _noop,
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
