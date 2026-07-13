import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import type { UseQueryOptions } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
import { Button } from '#/components/ui/button'
import { ResizablePanel, ResizablePanelGroup, ResizableHandle } from '#/components/ui/resizable'
import {
  orgWorkspacePrivateFileBrowserQueryOptions,
  orgWorkspaceSharedFileBrowserQueryOptions,
} from '#/lib/api/query'
import type { Workspace, WorkspaceFile, WorkspaceFileBrowserResult } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { useIde } from './useIdeStore'
import { SidebarPane } from './SidebarPane'
import { FileContextMenu } from './FileContextMenu'
import { Tip } from './schema-diagram/Tip'
import { CreateItemDialog } from './CreateItemDialog'
import { useFileActions } from './useFileActions'

type FilesPanelProps = {
  orgSlug: string
  workspace: Workspace
  maximized?: boolean
  onMaximizedChange?: (maximized: boolean) => void
}

type DialogState = { kind: 'file' | 'folder'; parentId: number | null } | null

export function FilesPanel({ orgSlug, workspace, maximized, onMaximizedChange }: FilesPanelProps) {
  const [dialogState, setDialogState] = useState<DialogState>(null)

  function openCreateDialog(kind: 'file' | 'folder', parentId: number | null) {
    setDialogState({ kind, parentId })
  }

  const headerActions = (
    <>
      <Tip label="New file">
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="New file"
          onClick={() => openCreateDialog('file', null)}
        >
          <Icon name="file-01" size={13} />
        </Button>
      </Tip>
      <Tip label="New folder">
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label="New folder"
          onClick={() => openCreateDialog('folder', null)}
        >
          <Icon name="folder-add" size={13} />
        </Button>
      </Tip>
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
        scroll={false}
      >
        <ResizablePanelGroup orientation="vertical" className="min-h-0 flex-1">
          <ResizablePanel defaultSize="60%" minSize="15%" className="overflow-hidden">
            <FilesSection
              orgSlug={orgSlug}
              workspace={workspace}
              visibility="private"
              title="My Files"
              onCreateFile={(parentId) => openCreateDialog('file', parentId)}
              onCreateFolder={(parentId) => openCreateDialog('folder', parentId)}
            />
          </ResizablePanel>
          <ResizableHandle withHandle />
          <ResizablePanel defaultSize="40%" minSize="15%" className="overflow-hidden">
            <FilesSection
              orgSlug={orgSlug}
              workspace={workspace}
              visibility="shared"
              title="Shared Files"
              onCreateFile={undefined}
              onCreateFolder={undefined}
            />
          </ResizablePanel>
        </ResizablePanelGroup>
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
  const actions = useFileActions(orgSlug, workspace, visibility)

  const queryOptions =
    visibility === 'private'
      ? orgWorkspacePrivateFileBrowserQueryOptions(orgSlug, workspace.id, null)
      : orgWorkspaceSharedFileBrowserQueryOptions(orgSlug, workspace.id, null)

  const { data, isLoading, isError, refetch } = useQuery(
    queryOptions as UseQueryOptions<WorkspaceFileBrowserResult>,
  )

  const children = data?.children ?? []

  const body = (
    <div className="flex flex-col py-1">
      {isLoading ? (
        <div className="px-3 py-2 text-xs text-muted-foreground">Loading...</div>
      ) : isError ? (
        <div className="flex items-center gap-2 px-3 py-2 text-xs text-muted-foreground">
          <span>Failed to load files.</span>
          <button
            type="button"
            onClick={() => void refetch()}
            className="font-medium text-primary hover:underline"
          >
            Retry
          </button>
        </div>
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
              activeFileId={actions.activeFileId}
              onOpenFile={actions.open}
              onOpenToSide={actions.openToSide}
              onSaveAs={actions.saveAs}
              onCreateFile={onCreateFile}
              onCreateFolder={onCreateFolder}
              onDelete={visibility === 'private' ? (id) => actions.deleteFile.mutate(id) : undefined}
            />
          ) : (
            <FileContextMenu
              key={file.id}
              kind="file"
              nodeId={file.id}
              nodeName={file.name}
              onOpen={() => actions.open(file)}
              onOpenToSide={() => actions.openToSide(file)}
              onSaveAs={() => actions.saveAs(file)}
              onDelete={visibility === 'private' ? () => actions.deleteFile.mutate(file.id) : undefined}
            >
              <FileTreeFile file={file} depth={0} active={file.id === actions.activeFileId} onOpen={actions.open} />
            </FileContextMenu>
          ),
        )
      )}
    </div>
  )

  return (
    <div className="flex h-full min-h-0 flex-col">
      {visibility === 'shared' ? (
        <div className="border-b border-border px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground select-none">
          {title}
        </div>
      ) : null}
      <div className="min-h-0 flex-1 overflow-y-auto overflow-x-hidden [scrollbar-width:thin]">
        {visibility === 'private' && onCreateFile && onCreateFolder ? (
          <FileContextMenu
            kind="root"
            className="min-h-full"
            onCreateFile={onCreateFile}
            onCreateFolder={onCreateFolder}
            onRefresh={actions.refresh}
          >
            {body}
          </FileContextMenu>
        ) : (
          body
        )}
      </div>
    </div>
  )
}

function FileTreeFolder({
  file,
  orgSlug,
  workspaceId,
  visibility,
  depth,
  activeFileId,
  onOpenFile,
  onOpenToSide,
  onSaveAs,
  onCreateFile,
  onCreateFolder,
  onDelete,
}: {
  file: WorkspaceFile
  orgSlug: string
  workspaceId: number
  visibility: 'private' | 'shared'
  depth: number
  activeFileId?: number | undefined
  onOpenFile: (file: WorkspaceFile) => void
  onOpenToSide: (file: WorkspaceFile) => void
  onSaveAs: (file: WorkspaceFile) => void
  onCreateFile?: ((parentId: number | null) => void) | undefined
  onCreateFolder?: ((parentId: number | null) => void) | undefined
  onDelete?: ((nodeId: number) => void) | undefined
}) {
  const nodeKey = `folder:${file.id}`
  const stored = useIde((s) => s.expandedNodes[nodeKey])
  const setNodeExpanded = useIde((s) => s.setNodeExpanded)
  const expanded = stored ?? false

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
      onClick={() => setNodeExpanded(nodeKey, !expanded)}
      style={{ paddingLeft: `${6 + depth * 11}px` }}
      className={cn(
        'mx-1 flex h-6 w-[calc(100%-0.5rem)] min-w-0 items-center gap-1.5 rounded-md pr-2 text-left text-xs',
        'transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
      )}
    >
      <Icon
        name={expanded ? 'chevron-down' : 'chevron-right'}
        size={11}
        className="shrink-0 text-muted-foreground"
      />
      <Icon
        name={expanded ? 'folder-open' : 'folder'}
        size={13}
        className="shrink-0 text-muted-foreground"
      />
      <span className="min-w-0 flex-1 truncate" title={file.name}>{file.name}</span>
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
              activeFileId={activeFileId}
              onOpenFile={onOpenFile}
              onOpenToSide={onOpenToSide}
              onSaveAs={onSaveAs}
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
              onOpenToSide={() => onOpenToSide(child)}
              onSaveAs={() => onSaveAs(child)}
              onDelete={onDelete ? () => onDelete(child.id) : undefined}
            >
              <FileTreeFile file={child} depth={depth + 1} active={child.id === activeFileId} onOpen={onOpenFile} />
            </FileContextMenu>
          ),
        )}
    </>
  )
}

function FileTreeFile({
  file,
  depth,
  active,
  onOpen,
}: {
  file: WorkspaceFile
  depth: number
  active?: boolean
  onOpen: (file: WorkspaceFile) => void
}) {
  return (
    <button
      type="button"
      onClick={() => onOpen(file)}
      style={{ paddingLeft: `${6 + depth * 11 + 14}px` }}
      className={cn(
        'mx-1 flex h-6 w-[calc(100%-0.5rem)] min-w-0 items-center gap-2 rounded-md pr-2 text-left text-xs transition-colors',
        active
          ? 'bg-primary/10 text-foreground hover:bg-primary/15'
          : 'hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
      )}
    >
      <Icon name="file-01" size={13} className="shrink-0 text-muted-foreground" />
      <span className="min-w-0 flex-1 truncate" title={file.name}>{file.name}</span>
    </button>
  )
}
