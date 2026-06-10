import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getPrivateWorkspaceFileContent } from '#/lib/api/files'
import type { EditorTab } from './useIdeStore'
import { useYDocRegistry } from './useYDocRegistry'

type UseFileContentOptions = {
  orgSlug: string
  workspaceId: number
  tab: EditorTab | undefined
  updateTabEtag: (tabId: string, etag: string) => void
}

export function useFileContent({
  orgSlug,
  workspaceId,
  tab,
  updateTabEtag,
}: UseFileContentOptions) {
  const registry = useYDocRegistry()
  const fileId = tab?.kind === 'file' ? tab.fileId : undefined
  // Always fetch file content when a file tab is open. The etag gates conflict
  // detection on saves, not content loading — if the page is refreshed after a
  // save the etag exists but the Y.Doc is empty and needs to be repopulated.
  const needsLoad = fileId != null

  const query = useQuery({
    queryKey: ['file-content', orgSlug, workspaceId, fileId],
    queryFn: () => getPrivateWorkspaceFileContent(orgSlug, workspaceId, fileId!),
    enabled: needsLoad,
    staleTime: Infinity,
    gcTime: 5 * 60 * 1000,
  })

  useEffect(() => {
    if (!query.data || !tab?.id) return

    // The tab has local unsaved changes backed by a Y.js snapshot that was
    // already applied to the Y.Doc during lifecycle init. Applying server
    // content here would overwrite those unsaved edits — skip it.
    if (tab.isDirty && tab.ySnapshot) return

    const doc = registry.getOrCreate(tab.id)
    const yText = doc.getText('content')

    // If the doc already has content, a peer window has already synced state
    // via BroadcastChannel full-state. Skip the text insert to avoid merging
    // two independently-created Y.js histories (which would double the content).
    if (yText.length === 0) {
      // No peer state yet — we are the first window. Initialize from server.
      // Origin 'server-load' causes the registry to broadcast our full state
      // so any other window that opens this file later can sync from us.
      doc.transact(() => {
        yText.insert(0, query.data.text)
      }, 'server-load')
    }

    // Only set the etag when the tab doesn't have one yet (initial load or
    // after the tab was re-opened). Re-applying it on every tab switch would
    // call updateTabEtag → isDirty: false, clearing the unsaved indicator.
    if (tab.etag === undefined) {
      updateTabEtag(tab.id, query.data.etag)
    }
  }, [query.data, tab?.id, tab?.etag, tab?.isDirty, tab?.ySnapshot, registry, updateTabEtag])

  return {
    isLoading: needsLoad && query.isLoading,
    isError: query.isError,
  }
}
