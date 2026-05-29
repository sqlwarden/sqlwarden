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
  nextConsoleNumber: number
  /** Live session IDs keyed by connectionId. A session entry means the backend has an open pool connection for this account. */
  sessions: Record<number, string>
  /** Last query result per tab ID. Not persisted to IndexedDB. */
  results: Record<string, QueryResult>
}

export type IdeActions = {
  setActiveWorkspace: (workspaceId: number) => void
  openTab: (tab: EditorTab) => void
  ensureTab: (tab: EditorTab) => void
  closeTab: (tabId: string) => void
  setActiveTab: (tabId: string) => void
  updateTabContent: (tabId: string, content: string) => void
  updateTabEtag: (tabId: string, etag: string) => void
  setTabConnection: (tabId: string, connectionId: number) => void
  setMaximizedPane: (pane: IdeState['maximizedPane']) => void
  setMaximizedSidebarPane: (pane: IdeState['maximizedSidebarPane']) => void
  setSidebarCollapsed: (collapsed: boolean) => void
  setSession: (connectionId: number, sessionId: string) => void
  clearSession: (connectionId: number) => void
  /** Replace the entire sessions map with authoritative data from the backend. */
  syncSessions: (backendSessions: Record<number, string>) => void
  setQueryResult: (tabId: string, result: QueryResult) => void
  /** Opens a new numbered console tab. Pass yState (encoded Y.Doc) so all windows
   *  that receive this tab share the same canonical Y.js initial history.
   *  Pass connectionId to pre-select a connection on the new tab. */
  openConsole: (workspace: Workspace, yState: number[], connectionId?: number) => void
}

// ─── Store factory ─────────────────────────────────────────────────────────────

function makeStorage(orgSlug: string): StateStorage {
  const key = `sqlwarden.ide.${orgSlug}`
  return {
    getItem: async () => {
      const val = await get<string>(key)
      return val ?? null
    },
    setItem: async (_name, value) => set(key, value),
    removeItem: async () => del(key),
  }
}

export function createIdeStore(orgSlug: string) {
  return createStore<IdeState & IdeActions>()(
    persist(
      (set) => ({
        activeWorkspaceId: undefined,
        maximizedPane: null,
        maximizedSidebarPane: null,
        sidebarCollapsed: false,
        activeTabId: undefined,
        tabs: [],
        nextConsoleNumber: 0,
        sessions: {},
        results: {},

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
            const nextTabs = s.tabs.filter((t) => t.id !== tabId)
            const nextActive =
              s.activeTabId === tabId
                ? nextTabs.find((t) => t.workspaceId === s.activeWorkspaceId)?.id
                : s.activeTabId
            const { [tabId]: _r, ...nextResults } = s.results
            return { tabs: nextTabs, activeTabId: nextActive, results: nextResults }
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

        setTabConnection: (tabId, connectionId) =>
          set((s) => ({ tabs: s.tabs.map((t) => (t.id === tabId ? { ...t, connectionId } : t)) })),

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

        openConsole: (workspace, yState, connectionId) =>
          set((s) => {
            const num = s.nextConsoleNumber + 1
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
              nextConsoleNumber: num,
              activeTabId: tab.id,
              tabs: exists ? s.tabs : [...s.tabs, tab],
            }
          }),
      }),
      {
        name: `sqlwarden.ide.${orgSlug}`,
        storage: createJSONStorage(() => makeStorage(orgSlug)),
        // Exclude ephemeral query results from IndexedDB — they can be large
        // and are meaningless after a page reload anyway.
        partialize: ({ results: _r, ...state }) => state,
      },
    ),
  )
}

// ─── React context + hook ──────────────────────────────────────────────────────

export type IdeStore = ReturnType<typeof createIdeStore>

export const IdeStoreContext = createContext<IdeStore | null>(null)

export function useIde<T>(selector: (state: IdeState & IdeActions) => T): T {
  const store = useContext(IdeStoreContext)
  if (!store) throw new Error('useIde must be used within WorkspaceIde')
  return useStore(store, selector)
}

// ─── Tab factory functions ──────────────────────────────────────────────────────

export const DEFAULT_CONSOLE_CONTENT = 'SELECT *\nFROM \nLIMIT 100;'

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
