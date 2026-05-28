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

    // Ensure the doc exists (getOrCreate is idempotent).
    const doc = registry.getOrCreate(tab.id)
    const yText = doc.getText('content')

    // Replace content atomically. Origin 'server-load' prevents the registry
    // from broadcasting this to other windows and prevents the Y.Doc observer
    // in EditorSection from marking the tab dirty.
    doc.transact(() => {
      yText.delete(0, yText.length)
      yText.insert(0, query.data.text)
    }, 'server-load')

    updateTabEtag(tab.id, query.data.etag)
  }, [query.data, tab?.id, registry, updateTabEtag])

  return {
    isLoading: needsLoad && query.isLoading,
    isError: query.isError,
  }
}
