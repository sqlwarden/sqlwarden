import { createContext, useContext, useState, type ReactNode } from 'react'

export type ConnectionLayout = 'flat' | 'grouped'
const KEY = 'sqlwarden.preference.connection_layout'

/** Pure reader: classic env-in-tree only for the exact 'grouped' value; else flat. */
export function readConnectionLayout(stored: string | null): ConnectionLayout {
  return stored === 'grouped' ? 'grouped' : 'flat'
}

type ConnectionLayoutContextValue = {
  connectionLayout: ConnectionLayout
  setConnectionLayout: (layout: ConnectionLayout) => void
}

const ConnectionLayoutContext = createContext<ConnectionLayoutContextValue>({
  connectionLayout: 'flat',
  setConnectionLayout: () => {},
})

/** Shares the localStorage-backed connection-layout preference across the app
 *  (Appearance settings + the explorer), so a change in one reflects in the other. */
export function ConnectionLayoutProvider({ children }: { children: ReactNode }) {
  const [layout, setLayoutState] = useState<ConnectionLayout>(() =>
    readConnectionLayout(localStorage.getItem(KEY)),
  )
  function setConnectionLayout(next: ConnectionLayout) {
    localStorage.setItem(KEY, next)
    setLayoutState(next)
  }
  return (
    <ConnectionLayoutContext.Provider value={{ connectionLayout: layout, setConnectionLayout }}>
      {children}
    </ConnectionLayoutContext.Provider>
  )
}

export function useConnectionLayout() {
  return useContext(ConnectionLayoutContext)
}
