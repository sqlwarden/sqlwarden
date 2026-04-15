import * as React from 'react'

type LayoutWidthMode = 'contracted' | 'expanded'

type LayoutWidthContextValue = {
  mode: LayoutWidthMode
  isExpanded: boolean
  toggleMode: () => void
}

const STORAGE_KEY = 'sqlwarden.layout_width'

const LayoutWidthContext = React.createContext<LayoutWidthContextValue | undefined>(undefined)

export function LayoutWidthProvider({ children }: { children: React.ReactNode }) {
  const [mode, setMode] = React.useState<LayoutWidthMode>(() => {
    const stored = window.localStorage.getItem(STORAGE_KEY)
    return stored === 'expanded' ? 'expanded' : 'contracted'
  })

  const toggleMode = React.useCallback(() => {
    setMode((current) => {
      const next = current === 'expanded' ? 'contracted' : 'expanded'
      window.localStorage.setItem(STORAGE_KEY, next)
      return next
    })
  }, [])

  const value = React.useMemo(
    () => ({
      mode,
      isExpanded: mode === 'expanded',
      toggleMode,
    }),
    [mode, toggleMode],
  )

  return <LayoutWidthContext.Provider value={value}>{children}</LayoutWidthContext.Provider>
}

export function useLayoutWidth() {
  const context = React.useContext(LayoutWidthContext)

  if (!context) {
    throw new Error('useLayoutWidth must be used within LayoutWidthProvider')
  }

  return context
}
