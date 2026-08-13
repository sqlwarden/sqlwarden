import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  deletePrivateWorkspaceFile,
  duplicateMyPrivateWorkspaceFile,
  duplicatePrivateWorkspaceFile,
  getPrivateWorkspaceFileContent,
  renameMyPrivateWorkspaceFile,
  renamePrivateWorkspaceFile,
  renameSharedWorkspaceFile,
} from '#/lib/api/files'
import { ApiError } from '#/lib/api/errors'
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
  const renameTabByFileId = useIde((state) => state.renameTabByFileId)
  const activeFileId = useIde((state) => {
    const id = selectActiveTabId(state, workspace.id)
    const tab = state.tabs.find((candidate) => candidate.id === id)
    return tab?.kind === 'file' ? tab.fileId : undefined
  })
  const queryClient = useQueryClient()
  const isPersonal = workspace.owner_type === 'space'

  const browserScope =
    visibility === 'private'
      ? isPersonal
        ? queryKeys.myWorkspacePrivateFileBrowserScope(workspace.id)
        : queryKeys.orgWorkspacePrivateFileBrowserScope(orgSlug, workspace.id)
      : queryKeys.orgWorkspaceSharedFileBrowserScope(orgSlug, workspace.id)
  const recentScope =
    visibility === 'private'
      ? isPersonal
        ? queryKeys.myWorkspacePrivateRecentFilesScope(workspace.id)
        : queryKeys.orgWorkspacePrivateRecentFilesScope(orgSlug, workspace.id)
      : queryKeys.orgWorkspaceSharedRecentFilesScope(orgSlug, workspace.id)

  function invalidateFileLists() {
    void queryClient.invalidateQueries({ queryKey: browserScope })
    void queryClient.invalidateQueries({ queryKey: recentScope })
  }

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

  const renameFile = useMutation({
    mutationFn: ({ nodeId, name }: { nodeId: number; name: string }) =>
      visibility === 'private'
        ? isPersonal
          ? renameMyPrivateWorkspaceFile(workspace.id, nodeId, name)
          : renamePrivateWorkspaceFile(orgSlug, workspace.id, nodeId, name)
        : renameSharedWorkspaceFile(orgSlug, workspace.id, nodeId, name),
    onSuccess: (file) => {
      invalidateFileLists()
      renameTabByFileId(file.id, file.name)
    },
    onError: (err) => {
      if (!(err instanceof ApiError && err.fieldErrors?.name)) {
        toast.error('Failed to rename.')
      }
    },
  })

  const duplicateFile = useMutation({
    mutationFn: ({ nodeId, name }: { nodeId: number; name: string }) =>
      isPersonal
        ? duplicateMyPrivateWorkspaceFile(workspace.id, nodeId, name)
        : duplicatePrivateWorkspaceFile(orgSlug, workspace.id, nodeId, name),
    onSuccess: () => {
      invalidateFileLists()
    },
    onError: (err) => {
      if (!(err instanceof ApiError && err.fieldErrors?.name)) {
        toast.error('Failed to duplicate.')
      }
    },
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
    const queryKey =
      visibility === 'private'
        ? queryKeys.orgWorkspacePrivateFileBrowserScope(orgSlug, workspace.id)
        : queryKeys.orgWorkspaceSharedFileBrowserScope(orgSlug, workspace.id)
    void queryClient.invalidateQueries({ queryKey })
  }

  return { activeFileId, deleteFile, renameFile, duplicateFile, open, openToSide, refresh, saveAs }
}
