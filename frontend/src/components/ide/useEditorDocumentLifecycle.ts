import { useEffect, useRef } from 'react'
import type { QueryClient } from '@tanstack/react-query'
import * as Y from 'yjs'
import { queryKeys } from '#/lib/api/query-keys'
import type { EditorTab } from './useIdeStore'
import type { YDocRegistry } from './useYDocRegistry'

export function useEditorDocumentLifecycle(tabs: EditorTab[], registry: YDocRegistry) {
  const trackedIdsRef = useRef(new Set<string>())

  useEffect(() => {
    const currentIds = new Set(tabs.map((tab) => tab.id))

    for (const tab of tabs) {
      if (trackedIdsRef.current.has(tab.id)) {
        continue
      }

      const initialState = tab.ySnapshot ?? tab.yState
      const initialContent = initialState || tab.kind === 'file' ? undefined : tab.content
      const doc = registry.getOrCreate(tab.id, initialContent)
      if (initialState && doc.getText('content').length === 0) {
        Y.applyUpdate(doc, new Uint8Array(initialState), tab.ySnapshot ? 'init' : 'server-load')
      }
    }

    for (const tabId of trackedIdsRef.current) {
      if (!currentIds.has(tabId)) {
        registry.destroy(tabId)
      }
    }

    trackedIdsRef.current = currentIds
  }, [registry, tabs])
}

export function useEditorSnapshotPersistence(
  tabs: EditorTab[],
  registry: YDocRegistry,
  updateTabContent: (tabId: string, content: string, snapshot?: number[]) => void,
) {
  useEffect(() => {
    const cleanups: Array<() => void> = []
    const timers = new Map<string, ReturnType<typeof setTimeout>>()

    for (const tab of tabs) {
      const doc = registry.get(tab.id)
      if (!doc) {
        continue
      }

      const observer = (_update: Uint8Array, origin: unknown) => {
        if (origin === 'server-load' || origin === 'init') {
          return
        }

        const currentTimer = timers.get(tab.id)
        if (currentTimer) {
          clearTimeout(currentTimer)
        }
        timers.set(
          tab.id,
          setTimeout(() => {
            updateTabContent(
              tab.id,
              doc.getText('content').toString(),
              Array.from(Y.encodeStateAsUpdate(doc)),
            )
            timers.delete(tab.id)
          }, 400),
        )
      }

      doc.on('update', observer)
      cleanups.push(() => {
        doc.off('update', observer)
        const timer = timers.get(tab.id)
        if (timer) {
          clearTimeout(timer)
        }
      })
    }

    return () => cleanups.forEach((cleanup) => cleanup())
  }, [registry, tabs, updateTabContent])
}

export function useClosedFileCacheCleanup(
  tabs: EditorTab[],
  orgSlug: string,
  queryClient: QueryClient,
) {
  const previousTabsRef = useRef<EditorTab[]>(tabs)

  useEffect(() => {
    const currentIds = new Set(tabs.map((tab) => tab.id))
    for (const tab of previousTabsRef.current) {
      if (!currentIds.has(tab.id) && tab.kind === 'file' && tab.fileId != null) {
        queryClient.removeQueries({
          queryKey: queryKeys.fileContent(orgSlug, tab.workspaceId, tab.fileId),
        })
      }
    }
    previousTabsRef.current = tabs
  }, [orgSlug, queryClient, tabs])
}

export function useEditorSaveShortcut(
  activeTab: EditorTab | undefined,
  saveEditorTab: (tab: EditorTab) => Promise<unknown>,
) {
  useEffect(() => {
    function handleKeyDown(event: KeyboardEvent) {
      if (!(event.metaKey || event.ctrlKey) || event.key.toLowerCase() !== 's') {
        return
      }

      event.preventDefault()
      if (activeTab?.kind === 'file' && activeTab.etag && activeTab.fileId) {
        void saveEditorTab(activeTab)
      }
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [activeTab, saveEditorTab])
}
