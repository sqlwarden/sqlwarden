import { createContext, useContext } from 'react'
import { createStore, useStore } from 'zustand'
import { persist, createJSONStorage, type StateStorage } from 'zustand/middleware'
import { get, set, del } from 'idb-keyval'
import type { Connection, Workspace, WorkspaceFile } from '#/lib/api/types'

// ─── Types ─────────────────────────────────────────────────────────────────────

export type TabKind = 'scratch' | 'file' | 'connection'

export type EditorTab = {
  id: string
  workspaceId: number
  title: string
  kind: TabKind
  subtitle?: string
  connectionId?: number
  content: string
}

export type IdeState = {
  activeWorkspaceId?: number
  maximizedPane: 'editor' | 'results' | null
  maximizedSidebarPane: 'database' | 'files' | null
  activeTabId?: string
  tabs: EditorTab[]
}

export type IdeActions = {
  setActiveWorkspace: (workspaceId: number) => void
  openTab: (tab: EditorTab) => void
  closeTab: (tabId: string) => void
  setActiveTab: (tabId: string) => void
  updateTabContent: (tabId: string, content: string) => void
  setTabConnection: (tabId: string, connectionId: number) => void
  setMaximizedPane: (pane: IdeState['maximizedPane']) => void
  setMaximizedSidebarPane: (pane: IdeState['maximizedSidebarPane']) => void
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
        activeTabId: undefined,
        tabs: [],

        setActiveWorkspace: (id) => set({ activeWorkspaceId: id }),

        openTab: (tab) =>
          set((s) => {
            const existing = s.tabs.find((t) => t.id === tab.id)
            return { activeTabId: tab.id, tabs: existing ? s.tabs : [...s.tabs, tab] }
          }),

        closeTab: (tabId) =>
          set((s) => {
            const nextTabs = s.tabs.filter((t) => t.id !== tabId)
            const nextActive =
              s.activeTabId === tabId
                ? nextTabs.find((t) => t.workspaceId === s.activeWorkspaceId)?.id
                : s.activeTabId
            return { tabs: nextTabs, activeTabId: nextActive }
          }),

        setActiveTab: (id) => set({ activeTabId: id }),

        updateTabContent: (tabId, content) =>
          set((s) => ({ tabs: s.tabs.map((t) => (t.id === tabId ? { ...t, content } : t)) })),

        setTabConnection: (tabId, connectionId) =>
          set((s) => ({ tabs: s.tabs.map((t) => (t.id === tabId ? { ...t, connectionId } : t)) })),

        setMaximizedPane: (pane) => set({ maximizedPane: pane }),
        setMaximizedSidebarPane: (pane) => set({ maximizedSidebarPane: pane }),
      }),
      {
        name: `sqlwarden.ide.${orgSlug}`,
        storage: createJSONStorage(() => makeStorage(orgSlug)),
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

export function newScratchTab(workspace: Workspace): EditorTab {
  const ts = Date.now()
  return {
    id: `scratch:${workspace.id}:${ts}`,
    workspaceId: workspace.id,
    title: `Console ${new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`,
    kind: 'scratch',
    content: 'SELECT *\nFROM \nLIMIT 100;',
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
    content: `-- ${file.name}\n-- File content loading is not yet implemented.\n`,
  }
}
