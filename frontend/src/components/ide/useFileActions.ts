import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { deletePrivateWorkspaceFile, getPrivateWorkspaceFileContent } from '#/lib/api/files'
import { queryKeys } from '#/lib/api/query-keys'
import type { Workspace, WorkspaceFile } from '#/lib/api/types'
import { saveTextAs } from './saveFile'
import { activeTabId as selectActiveTabId, newFileTab, useIde } from './useIdeStore'

export function useFileActions(
  orgSlug: string,
  workspace: Workspace,
  visibility: 'private' | 'shared',
) {
  const openTab = useIde((state) => state.openTab)
  const openTabToSide = useIde((state) => state.openTabToSide)
  const closeTab = useIde((state) => state.closeTab)
  const activeFileId = useIde((state) => {
    const id = selectActiveTabId(state, workspace.id)
    const tab = state.tabs.find((candidate) => candidate.id === id)
    return tab?.kind === 'file' ? tab.fileId : undefined
  })
  const queryClient = useQueryClient()

  const deleteFile = useMutation({
    mutationFn: (nodeId: number) => deletePrivateWorkspaceFile(orgSlug, workspace.id, nodeId),
    onSuccess: (_, nodeId) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.orgWorkspacePrivateFileBrowserScope(orgSlug, workspace.id),
      })
      closeTab(`file:${nodeId}`)
    },
    onError: () => toast.error('Failed to delete file.'),
  })

  function open(file: WorkspaceFile) {
    openTab(newFileTab(file, workspace))
  }

  function openToSide(file: WorkspaceFile) {
    openTabToSide(newFileTab(file, workspace))
  }

  async function saveAs(file: WorkspaceFile) {
    try {
      const { text } = await getPrivateWorkspaceFileContent(orgSlug, workspace.id, file.id)
      saveTextAs(file.name, text)
    } catch {
      toast.error('Failed to save file.')
    }
  }

  function refresh() {
    const queryKey = visibility === 'private'
      ? queryKeys.orgWorkspacePrivateFileBrowserScope(orgSlug, workspace.id)
      : queryKeys.orgWorkspaceSharedFileBrowserScope(orgSlug, workspace.id)
    void queryClient.invalidateQueries({ queryKey })
  }

  return { activeFileId, deleteFile, open, openToSide, refresh, saveAs }
}
