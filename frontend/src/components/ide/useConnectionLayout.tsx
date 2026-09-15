import { createContext, useContext, useState, type ReactNode } from 'react'
import type { Layout } from 'react-resizable-panels'

const GROUP_KEY = 'sqlwarden.preference.connection_group_by_environment'
const SPLIT_KEY = 'sqlwarden.preference.connection_split_view'
const SPLIT_LAYOUT_KEY = 'sqlwarden.preference.connection_split_layout'
const TOP_COLLAPSED_KEY = 'sqlwarden.preference.connection_top_collapsed'
const BOTTOM_COLLAPSED_KEY = 'sqlwarden.preference.connection_bottom_collapsed'
const SIDEBAR_LAYOUT_KEY = 'sqlwarden.preference.ide_sidebar_layout'
const EDITOR_RESULTS_LAYOUT_KEY = 'sqlwarden.preference.ide_editor_results_layout'

/** Pure reader: only the exact stored `'false'` opts out; anything else
 *  (including unset) keeps the split+grouped default. */
export function readBooleanPreference(stored: string | null): boolean {
  return stored !== 'false'
}

/** Parses a persisted panel-group layout, tolerating anything malformed or
 *  absent (a fresh install, a manually edited localStorage, a schema change)
 *  by falling back to the resizable-panels library's own auto-sizing. */
export function readLayoutPreference(stored: string | null): Layout | undefined {
  if (!stored) return undefined
  try {
    const parsed: unknown = JSON.parse(stored)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return undefined
    if (!Object.values(parsed).every((v) => typeof v === 'number')) return undefined
    return parsed as Layout
  } catch {
    return undefined
  }
}

type ConnectionLayoutContextValue = {
  groupByEnvironment: boolean
  setGroupByEnvironment: (value: boolean) => void
  splitView: boolean
  setSplitView: (value: boolean) => void
  explorerLayout: Layout | undefined
  setExplorerLayout: (layout: Layout) => void
  explorerTopCollapsed: boolean
  setExplorerTopCollapsed: (value: boolean) => void
  explorerBottomCollapsed: boolean
  setExplorerBottomCollapsed: (value: boolean) => void
  sidebarLayout: Layout | undefined
  setSidebarLayout: (layout: Layout) => void
  editorResultsLayout: Layout | undefined
  setEditorResultsLayout: (layout: Layout) => void
}

const ConnectionLayoutContext = createContext<ConnectionLayoutContextValue>({
  groupByEnvironment: true,
  setGroupByEnvironment: () => {},
  splitView: true,
  setSplitView: () => {},
  explorerLayout: undefined,
  setExplorerLayout: () => {},
  explorerTopCollapsed: false,
  setExplorerTopCollapsed: () => {},
  explorerBottomCollapsed: false,
  setExplorerBottomCollapsed: () => {},
  sidebarLayout: undefined,
  setSidebarLayout: () => {},
  editorResultsLayout: undefined,
  setEditorResultsLayout: () => {},
})

/** Shares the localStorage-backed connection-layout preferences across the
 *  app (Appearance settings + the explorer), so a change in one reflects in
 *  the other. Group-by-environment and split-view are independent toggles
 *  (2x2 matrix), both defaulting to on. The split ratio and each pane's
 *  collapsed/maximized state persist here too, so the explorer's layout
 *  survives navigation and page refresh instead of resetting on remount. */
export function ConnectionLayoutProvider({ children }: { children: ReactNode }) {
  const [groupByEnvironment, setGroupState] = useState(() =>
    readBooleanPreference(localStorage.getItem(GROUP_KEY)),
  )
  const [splitView, setSplitState] = useState(() =>
    readBooleanPreference(localStorage.getItem(SPLIT_KEY)),
  )
  const [explorerLayout, setLayoutState] = useState<Layout | undefined>(() =>
    readLayoutPreference(localStorage.getItem(SPLIT_LAYOUT_KEY)),
  )
  const [explorerTopCollapsed, setTopCollapsedState] = useState(
    () => localStorage.getItem(TOP_COLLAPSED_KEY) === 'true',
  )
  const [explorerBottomCollapsed, setBottomCollapsedState] = useState(
    () => localStorage.getItem(BOTTOM_COLLAPSED_KEY) === 'true',
  )
  const [sidebarLayout, setSidebarLayoutState] = useState<Layout | undefined>(() =>
    readLayoutPreference(localStorage.getItem(SIDEBAR_LAYOUT_KEY)),
  )
  const [editorResultsLayout, setEditorResultsLayoutState] = useState<Layout | undefined>(() =>
    readLayoutPreference(localStorage.getItem(EDITOR_RESULTS_LAYOUT_KEY)),
  )
  function setGroupByEnvironment(value: boolean) {
    localStorage.setItem(GROUP_KEY, String(value))
    setGroupState(value)
  }
  function setSplitView(value: boolean) {
    localStorage.setItem(SPLIT_KEY, String(value))
    setSplitState(value)
  }
  function setExplorerLayout(layout: Layout) {
    localStorage.setItem(SPLIT_LAYOUT_KEY, JSON.stringify(layout))
    setLayoutState(layout)
  }
  function setExplorerTopCollapsed(value: boolean) {
    localStorage.setItem(TOP_COLLAPSED_KEY, String(value))
    setTopCollapsedState(value)
  }
  function setExplorerBottomCollapsed(value: boolean) {
    localStorage.setItem(BOTTOM_COLLAPSED_KEY, String(value))
    setBottomCollapsedState(value)
  }
  function setSidebarLayout(layout: Layout) {
    localStorage.setItem(SIDEBAR_LAYOUT_KEY, JSON.stringify(layout))
    setSidebarLayoutState(layout)
  }
  function setEditorResultsLayout(layout: Layout) {
    localStorage.setItem(EDITOR_RESULTS_LAYOUT_KEY, JSON.stringify(layout))
    setEditorResultsLayoutState(layout)
  }
  return (
    <ConnectionLayoutContext.Provider
      value={{
        groupByEnvironment,
        setGroupByEnvironment,
        splitView,
        setSplitView,
        explorerLayout,
        setExplorerLayout,
        explorerTopCollapsed,
        setExplorerTopCollapsed,
        explorerBottomCollapsed,
        setExplorerBottomCollapsed,
        sidebarLayout,
        setSidebarLayout,
        editorResultsLayout,
        setEditorResultsLayout,
      }}
    >
      {children}
    </ConnectionLayoutContext.Provider>
  )
}

export function useConnectionLayout() {
  return useContext(ConnectionLayoutContext)
}
