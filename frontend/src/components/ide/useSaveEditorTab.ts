import { useCallback } from 'react'
import { toast } from 'sonner'
import { updatePrivateWorkspaceFileContent } from '#/lib/api/files'
import { saveEditorTab } from './filePersistence'
import { useIde, type EditorTab } from './useIdeStore'
import { useYDocRegistry } from './useYDocRegistry'

export function useSaveEditorTab(orgSlug: string, workspaceId: number) {
  const registry = useYDocRegistry()
  const updateTabEtag = useIde((state) => state.updateTabEtag)

  return useCallback(
    async (tab: EditorTab) => {
      try {
        return await saveEditorTab(tab, {
          readContent: (currentTab) => {
            const doc = registry.get(currentTab.id)
            return doc ? doc.getText('content').toString() : currentTab.content
          },
          updateFile: (fileId, content, etag) =>
            updatePrivateWorkspaceFileContent(orgSlug, workspaceId, fileId, content, etag),
          updateTabEtag,
        })
      } catch (error) {
        const status = (error as { status?: number }).status
        toast.error(
          status === 412 || status === 409
            ? 'File changed externally. Reload before saving.'
            : 'Failed to save file.',
        )
        return undefined
      }
    },
    [orgSlug, registry, updateTabEtag, workspaceId],
  )
}
