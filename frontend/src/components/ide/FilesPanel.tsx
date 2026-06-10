import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type { UseQueryOptions } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { Button } from '#/components/ui/button'
import { Separator } from '#/components/ui/separator'
import {
  orgWorkspacePrivateFileBrowserQueryOptions,
  orgWorkspaceSharedFileBrowserQueryOptions,
} from '#/lib/api/query'
import type { Workspace, WorkspaceFile, WorkspaceFileBrowserResult } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { useIde, newFileTab } from './useIdeStore'
import { deletePrivateWorkspaceFile } from '#/lib/api/files'
import { SidebarPane } from './SidebarPane'
import { FileContextMenu } from './FileContextMenu'
import { CreateItemDialog } from './CreateItemDialog'

type FilesPanelProps = {
  orgSlug: string
  workspace: Workspace
  maximized: boolean
  onMaximizedChange: (maximized: boolean) => void
}

type DialogState = { kind: 'file' | 'folder'; parentId: number | null } | null

export function FilesPanel({ orgSlug, workspace, maximized, onMaximizedChange }: FilesPanelProps) {
  const [dialogState, setDialogState] = useState<DialogState>(null)

  function openCreateDialog(kind: 'file' | 'folder', parentId: number | null) {
    setDialogState({ kind, parentId })
  }

  const headerActions = (
    <>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label="New file"
        onClick={() => openCreateDialog('file', null)}
      >
        <Icon name="file-01" size={13} />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label="New folder"
        onClick={() => openCreateDialog('folder', null)}
      >
        <Icon name="folder-add" size={13} />
      </Button>
    </>
  )

  return (
    <>
      <SidebarPane
        title="Files"
        icon="folder-open"
        maximized={maximized}
        onMaximizedChange={onMaximizedChange}
        actions={headerActions}
      >
        <FilesSection
          orgSlug={orgSlug}
          workspace={workspace}
          visibility="private"
          title="My Files"
          onCreateFile={(parentId) => openCreateDialog('file', parentId)}
          onCreateFolder={(parentId) => openCreateDialog('folder', parentId)}
        />
        <Separator className="my-1" />
        <FilesSection
          orgSlug={orgSlug}
          workspace={workspace}
          visibility="shared"
          title="Shared Files"
          onCreateFile={undefined}
          onCreateFolder={undefined}
        />
      </SidebarPane>

      {dialogState && (
        <CreateItemDialog
          open={true}
          onOpenChange={(open) => { if (!open) setDialogState(null) }}
          kind={dialogState.kind}
          parentId={dialogState.parentId}
          orgSlug={orgSlug}
          workspaceId={workspace.id}
          onSuccess={() => setDialogState(null)}
        />
      )}
    </>
  )
}

function FilesSection({
  orgSlug,
  workspace,
  visibility,
  title,
  onCreateFile,
  onCreateFolder,
}: {
  orgSlug: string
  workspace: Workspace
  visibility: 'private' | 'shared'
  title: string
  onCreateFile?: ((parentId: number | null) => void) | undefined
  onCreateFolder?: ((parentId: number | null) => void) | undefined
}) {
  const openTab = useIde((s) => s.openTab)
  const closeTab = useIde((s) => s.closeTab)
  const queryClient = useQueryClient()

  const queryOptions =
    visibility === 'private'
      ? orgWorkspacePrivateFileBrowserQueryOptions(orgSlug, workspace.id, null)
      : orgWorkspaceSharedFileBrowserQueryOptions(orgSlug, workspace.id, null)

  const { data, isLoading, isError } = useQuery(
    queryOptions as UseQueryOptions<WorkspaceFileBrowserResult>,
  )

  const deleteMutation = useMutation({
    mutationFn: (nodeId: number) =>
      deletePrivateWorkspaceFile(orgSlug, workspace.id, nodeId),
    onSuccess: (_, nodeId) => {
      // Invalidate all browser queries for this workspace so every open folder refreshes
      queryClient.invalidateQueries({
        queryKey: ['org-workspace-private-file-browser', orgSlug, workspace.id],
      })
      // Close the tab if this file was open
      closeTab(`file:${nodeId}`)
    },
  })

  function handleOpenFile(file: WorkspaceFile) {
    openTab(newFileTab(file, workspace))
  }

  const children = data?.children ?? []

  const sectionLabel = (
    <div className="px-2 py-1.5 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground select-none">
      {title}
    </div>
  )

  return (
    <div className="flex flex-col">
      {visibility === 'private' && onCreateFile ? (
        <FileContextMenu
          kind="root"
          onCreateFile={onCreateFile}
          onCreateFolder={onCreateFolder}
        >
          {sectionLabel}
        </FileContextMenu>
      ) : (
        sectionLabel
      )}
      {isLoading ? (
        <div className="px-3 py-2 text-xs text-muted-foreground">Loading...</div>
      ) : isError ? (
        <div className="px-3 py-2 text-xs text-muted-foreground">Failed to load files.</div>
      ) : children.length === 0 ? (
        <div className="px-3 py-2 text-xs text-muted-foreground">No files yet.</div>
      ) : (
        children.map((file) =>
          file.object_type === 'folder' ? (
            <FileTreeFolder
              key={file.id}
              file={file}
              orgSlug={orgSlug}
              workspaceId={workspace.id}
              visibility={visibility}
              depth={0}
              onOpenFile={handleOpenFile}
              onCreateFile={onCreateFile}
              onCreateFolder={onCreateFolder}
              onDelete={visibility === 'private' ? (id) => deleteMutation.mutate(id) : undefined}
            />
          ) : (
            <FileContextMenu
              key={file.id}
              kind="file"
              nodeId={file.id}
              nodeName={file.name}
              onOpen={() => handleOpenFile(file)}
              onDelete={visibility === 'private' ? () => deleteMutation.mutate(file.id) : undefined}
            >
              <FileTreeFile file={file} depth={0} onOpen={handleOpenFile} />
            </FileContextMenu>
          ),
        )
      )}
    </div>
  )
}

function FileTreeFolder({
  file,
  orgSlug,
  workspaceId,
  visibility,
  depth,
  onOpenFile,
  onCreateFile,
  onCreateFolder,
  onDelete,
}: {
  file: WorkspaceFile
  orgSlug: string
  workspaceId: number
  visibility: 'private' | 'shared'
  depth: number
  onOpenFile: (file: WorkspaceFile) => void
  onCreateFile?: ((parentId: number | null) => void) | undefined
  onCreateFolder?: ((parentId: number | null) => void) | undefined
  onDelete?: ((nodeId: number) => void) | undefined
}) {
  const [expanded, setExpanded] = useState(false)

  const queryOptions =
    visibility === 'private'
      ? orgWorkspacePrivateFileBrowserQueryOptions(orgSlug, workspaceId, file.id)
      : orgWorkspaceSharedFileBrowserQueryOptions(orgSlug, workspaceId, file.id)

  const { data } = useQuery({
    ...queryOptions,
    enabled: expanded,
  } as UseQueryOptions<WorkspaceFileBrowserResult>)

  const children = data?.children ?? []

  const folderRow = (
    <button
      type="button"
      onClick={() => setExpanded((v) => !v)}
      style={{ paddingLeft: `${8 + depth * 12}px` }}
      className={cn(
        'flex h-7 w-full items-center gap-1.5 pr-2 text-left text-xs',
        'transition-colors hover:bg-accent hover:text-accent-foreground',
      )}
    >
      <Icon
        name={expanded ? 'arrow-down-01' : 'arrow-right-01'}
        size={11}
        className="shrink-0 text-muted-foreground"
      />
      <Icon
        name={expanded ? 'folder-open' : 'folder'}
        size={14}
        className="shrink-0 text-muted-foreground"
      />
      <span className="min-w-0 flex-1 truncate">{file.name}</span>
    </button>
  )

  return (
    <>
      {visibility === 'private' && onCreateFile ? (
        <FileContextMenu
          kind="folder"
          nodeId={file.id}
          nodeName={file.name}
          onCreateFile={onCreateFile}
          onCreateFolder={onCreateFolder}
          onDelete={onDelete ? () => onDelete(file.id) : undefined}
        >
          {folderRow}
        </FileContextMenu>
      ) : (
        folderRow
      )}

      {expanded &&
        children.map((child) =>
          child.object_type === 'folder' ? (
            <FileTreeFolder
              key={child.id}
              file={child}
              orgSlug={orgSlug}
              workspaceId={workspaceId}
              visibility={visibility}
              depth={depth + 1}
              onOpenFile={onOpenFile}
              onCreateFile={onCreateFile}
              onCreateFolder={onCreateFolder}
              onDelete={onDelete}
            />
          ) : (
            <FileContextMenu
              key={child.id}
              kind="file"
              nodeId={child.id}
              nodeName={child.name}
              onOpen={() => onOpenFile(child)}
              onDelete={onDelete ? () => onDelete(child.id) : undefined}
            >
              <FileTreeFile file={child} depth={depth + 1} onOpen={onOpenFile} />
            </FileContextMenu>
          ),
        )}
    </>
  )
}

function FileTreeFile({
  file,
  depth,
  onOpen,
}: {
  file: WorkspaceFile
  depth: number
  onOpen: (file: WorkspaceFile) => void
}) {
  return (
    <button
      type="button"
      onClick={() => onOpen(file)}
      style={{ paddingLeft: `${8 + depth * 12 + 15}px` }}
      className={cn(
        'flex h-7 w-full items-center gap-2 pr-2 text-left text-xs',
        'transition-colors hover:bg-accent hover:text-accent-foreground',
      )}
    >
      <Icon name="file-01" size={13} className="shrink-0 text-muted-foreground" />
      <span className="min-w-0 flex-1 truncate">{file.name}</span>
    </button>
  )
}
