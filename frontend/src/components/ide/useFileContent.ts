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
  const needsLoad = fileId != null && !tab?.etag

  const query = useQuery({
    queryKey: ['file-content', orgSlug, workspaceId, fileId],
    queryFn: () => getPrivateWorkspaceFileContent(orgSlug, workspaceId, fileId!),
    enabled: needsLoad,
    staleTime: Infinity,
    gcTime: 0,
  })

  useEffect(() => {
    if (!query.data || !tab?.id) return

    const doc = registry.getOrCreate(tab.id)
    const yText = doc.getText('content')

    // If the doc already has content, a peer window has already synced state
    // via BroadcastChannel full-state. Skip the text insert to avoid merging
    // two independently-created Y.js histories (which would double the content).
    // Always update the etag so dirty tracking and saves work correctly.
    if (yText.length === 0) {
      // No peer state yet — we are the first window. Initialize from server.
      // Origin 'server-load' causes the registry to broadcast our full state
      // so any other window that opens this file later can sync from us.
      doc.transact(() => {
        yText.insert(0, query.data.text)
      }, 'server-load')
    }

    updateTabEtag(tab.id, query.data.etag)
  }, [query.data, tab?.id, registry, updateTabEtag])

  return {
    isLoading: needsLoad && query.isLoading,
    isError: query.isError,
  }
}
