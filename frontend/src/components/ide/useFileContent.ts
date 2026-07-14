import { useEffect } from 'react'
import { queryKeys } from '#/lib/api/query-keys'
import { useQuery } from '@tanstack/react-query'
import { getPrivateWorkspaceFileContent } from '#/lib/api/files'
import type { EditorTab } from './useIdeStore'
import { useYDocRegistry } from './useYDocRegistry'
import { hydrateFileTab } from './fileHydration'

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
    queryKey: queryKeys.fileContent(orgSlug, workspaceId, fileId),
    queryFn: () => getPrivateWorkspaceFileContent(orgSlug, workspaceId, fileId!),
    enabled: needsLoad,
    staleTime: Infinity,
    gcTime: 5 * 60 * 1000,
  })

  useEffect(() => {
    if (!query.data || !tab?.id) return

    hydrateFileTab(tab, query.data, registry, updateTabEtag)
  }, [query.data, tab, registry, updateTabEtag])

  return {
    isLoading: needsLoad && query.isLoading,
    isError: query.isError,
    retry: () => void query.refetch(),
  }
}
