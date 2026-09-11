import { createContext, useContext, useState, type ReactNode } from 'react'

const GROUP_KEY = 'sqlwarden.preference.connection_group_by_environment'
const SPLIT_KEY = 'sqlwarden.preference.connection_split_view'

/** Pure reader: only the exact stored `'false'` opts out; anything else
 *  (including unset) keeps the split+grouped default. */
export function readBooleanPreference(stored: string | null): boolean {
  return stored !== 'false'
}

type ConnectionLayoutContextValue = {
  groupByEnvironment: boolean
  setGroupByEnvironment: (value: boolean) => void
  splitView: boolean
  setSplitView: (value: boolean) => void
}

const ConnectionLayoutContext = createContext<ConnectionLayoutContextValue>({
  groupByEnvironment: true,
  setGroupByEnvironment: () => {},
  splitView: true,
  setSplitView: () => {},
})

/** Shares the localStorage-backed connection-layout preferences across the
 *  app (Appearance settings + the explorer), so a change in one reflects in
 *  the other. Group-by-environment and split-view are independent toggles
 *  (2x2 matrix), both defaulting to on. */
export function ConnectionLayoutProvider({ children }: { children: ReactNode }) {
  const [groupByEnvironment, setGroupState] = useState(() =>
    readBooleanPreference(localStorage.getItem(GROUP_KEY)),
  )
  const [splitView, setSplitState] = useState(() =>
    readBooleanPreference(localStorage.getItem(SPLIT_KEY)),
  )
  function setGroupByEnvironment(value: boolean) {
    localStorage.setItem(GROUP_KEY, String(value))
    setGroupState(value)
  }
  function setSplitView(value: boolean) {
    localStorage.setItem(SPLIT_KEY, String(value))
    setSplitState(value)
  }
  return (
    <ConnectionLayoutContext.Provider
      value={{ groupByEnvironment, setGroupByEnvironment, splitView, setSplitView }}
    >
      {children}
    </ConnectionLayoutContext.Provider>
  )
}

export function useConnectionLayout() {
  return useContext(ConnectionLayoutContext)
}
