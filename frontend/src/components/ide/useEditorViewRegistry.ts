import { createContext, useContext } from 'react'
import type { EditorView } from '@codemirror/view'

export type EditorViewRegistry = {
  register: (tabId: string, view: EditorView) => void
  unregister: (tabId: string) => void
  get: (tabId: string) => EditorView | undefined
}

export function createEditorViewRegistry(): EditorViewRegistry {
  const views = new Map<string, EditorView>()
  return {
    register: (tabId, view) => views.set(tabId, view),
    unregister: (tabId) => views.delete(tabId),
    get: (tabId) => views.get(tabId),
  }
}

export const EditorViewRegistryContext = createContext<EditorViewRegistry | null>(null)

export function useEditorViewRegistry(): EditorViewRegistry {
  const ctx = useContext(EditorViewRegistryContext)
  if (!ctx) throw new Error('useEditorViewRegistry must be used within an EditorViewRegistryContext.Provider')
  return ctx
}
