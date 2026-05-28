import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getPrivateWorkspaceFileContent } from '#/lib/api/files'
import type { EditorTab } from './useIdeStore'

type UseFileContentOptions = {
  orgSlug: string
  workspaceId: number
  tab: EditorTab | undefined
  updateTabContent: (tabId: string, content: string) => void
  updateTabEtag: (tabId: string, etag: string) => void
}

export function useFileContent({
  orgSlug,
  workspaceId,
  tab,
  updateTabContent,
  updateTabEtag,
}: UseFileContentOptions) {
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
    if (query.data && tab?.id) {
      updateTabContent(tab.id, query.data.text)
      updateTabEtag(tab.id, query.data.etag)
    }
  }, [query.data, tab?.id, updateTabContent, updateTabEtag])

  return {
    isLoading: needsLoad && query.isLoading,
    isError: query.isError,
  }
}
