import { createContext, useContext } from 'react'
import type { EditorView } from '@codemirror/view'

export type TabViewState = {
  selection: { anchor: number; head: number }
  scroll: ReturnType<EditorView['scrollSnapshot']>
}

export type TabViewStateCache = {
  save: (viewKey: string, state: TabViewState) => void
  load: (viewKey: string) => TabViewState | undefined
}

export function createTabViewStateCache(): TabViewStateCache {
  const states = new Map<string, TabViewState>()
  return {
    save: (viewKey, state) => states.set(viewKey, state),
    load: (viewKey) => states.get(viewKey),
  }
}

export const TabViewStateCacheContext = createContext<TabViewStateCache | null>(null)

export function useTabViewStateCache(): TabViewStateCache {
  const ctx = useContext(TabViewStateCacheContext)
  if (!ctx)
    throw new Error('useTabViewStateCache must be used within a TabViewStateCacheContext.Provider')
  return ctx
}
