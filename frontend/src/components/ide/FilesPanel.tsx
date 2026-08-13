import { useEffect, useId, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import type { UseQueryOptions } from '@tanstack/react-query'
import { Icon, type AppIcon } from '#/lib/icons'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { ResizablePanel, ResizablePanelGroup, ResizableHandle } from '#/components/ui/resizable'
import {
  orgWorkspacePrivateFileBrowserQueryOptions,
  orgWorkspaceSharedFileBrowserQueryOptions,
} from '#/lib/api/query'
import { ApiError } from '#/lib/api/errors'
import type { Workspace, WorkspaceFile, WorkspaceFileBrowserResult } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { useIde } from './useIdeStore'
import { SidebarPane } from './SidebarPane'
import { FileContextMenu } from './FileContextMenu'
import { FileNameDialog, selectionRange } from './FileNameDialog'
import { Tip } from './schema-diagram/Tip'
import { CreateItemDialog } from './CreateItemDialog'
import { useFileActions } from './useFileActions'
import { workspaceFileIcon } from './workspaceFileIcon'

type FilesPanelProps = {
  orgSlug: string
  workspace: Workspace
  maximized?: boolean
  onMaximizedChange?: (maximized: boolean) => void
}

type DialogState = { kind: 'file' | 'folder'; parentId: number | null } | null

type DuplicateState = { file: WorkspaceFile } | null

/** Props threaded through the private file tree so exactly one row can be
 *  edited inline at a time. */
type RenameControls = {
  renamingId: number | null
  renamePending: boolean
  renameError: string | null
  onRenameSubmit: (nodeId: number, name: string) => void
  onRenameCancel: () => void
  onClearRenameError: () => void
}

export function FilesPanel({ orgSlug, workspace, maximized, onMaximizedChange }: FilesPanelProps) {
  const [dialogState, setDialogState] = useState<DialogState>(null)
  const [renamingId, setRenamingId] = useState<number | null>(null)
  const [duplicateState, setDuplicateState] = useState<DuplicateState>(null)

  const privateActions = useFileActions(orgSlug, workspace, 'private')
  const sharedActions = useFileActions(orgSlug, workspace, 'shared')

  function openCreateDialog(kind: 'file' | 'folder', parentId: number | null) {
    setDialogState({ kind, parentId })
  }

  function startRename(file: WorkspaceFile) {
    privateActions.renameFile.reset()
    setRenamingId(file.id)
  }

  function openDuplicateDialog(file: WorkspaceFile) {
    privateActions.duplicateFile.reset()
    setDuplicateState({ file })
  }

  const renameFieldError =
    privateActions.renameFile.error instanceof ApiError
      ? (privateActions.renameFile.error.fieldErrors?.name ?? null)
      : null

  function handleRenameSubmit(nodeId: number, name: string) {
    privateActions.renameFile.mutate({ nodeId, name }, { onSuccess: () => setRenamingId(null) })
  }

  function handleRenameCancel() {
    if (privateActions.renameFile.isPending) return
    privateActions.renameFile.reset()
    setRenamingId(null)
  }

  const renameControls: RenameControls = {
    renamingId,
    renamePending: privateActions.renameFile.isPending,
    renameError: renameFieldError,
    onRenameSubmit: handleRenameSubmit,
    onRenameCancel: handleRenameCancel,
    onClearRenameError: () => privateActions.renameFile.reset(),
  }

  const duplicateFieldError =
    privateActions.duplicateFile.error instanceof ApiError
      ? (privateActions.duplicateFile.error.fieldErrors?.name ?? null)
      : null

  function handleDuplicateSubmit(name: string) {
    if (!duplicateState) return
    privateActions.duplicateFile.mutate(
      { nodeId: duplicateState.file.id, name },
      { onSuccess: () => setDuplicateState(null) },
    )
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
              actions={privateActions}
              onCreateFile={(parentId) => openCreateDialog('file', parentId)}
              onCreateFolder={(parentId) => openCreateDialog('folder', parentId)}
              onRename={startRename}
              onDuplicate={openDuplicateDialog}
              renameControls={renameControls}
            />
          </ResizablePanel>
          <ResizableHandle withHandle />
          <ResizablePanel defaultSize="40%" minSize="15%" className="overflow-hidden">
            <FilesSection
              orgSlug={orgSlug}
              workspace={workspace}
              visibility="shared"
              title="Shared Files"
              actions={sharedActions}
              onCreateFile={undefined}
              onCreateFolder={undefined}
              onRename={undefined}
              onDuplicate={undefined}
              renameControls={undefined}
            />
          </ResizablePanel>
        </ResizablePanelGroup>
      </SidebarPane>

      {dialogState && (
        <CreateItemDialog
          open={true}
          onOpenChange={(open) => {
            if (!open) setDialogState(null)
          }}
          kind={dialogState.kind}
          parentId={dialogState.parentId}
          orgSlug={orgSlug}
          workspaceId={workspace.id}
          onSuccess={() => setDialogState(null)}
        />
      )}

      {duplicateState && (
        <FileNameDialog
          open={true}
          onOpenChange={(open) => {
            if (!open) setDuplicateState(null)
          }}
          objectType={duplicateState.file.object_type}
          currentName={duplicateState.file.name}
          pending={privateActions.duplicateFile.isPending}
          fieldError={duplicateFieldError}
          onClearError={() => privateActions.duplicateFile.reset()}
          onSubmit={handleDuplicateSubmit}
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
  actions,
  onCreateFile,
  onCreateFolder,
  onRename,
  onDuplicate,
  renameControls,
}: {
  orgSlug: string
  workspace: Workspace
  visibility: 'private' | 'shared'
  title: string
  actions: ReturnType<typeof useFileActions>
  onCreateFile?: ((parentId: number | null) => void) | undefined
  onCreateFolder?: ((parentId: number | null) => void) | undefined
  onRename?: ((file: WorkspaceFile) => void) | undefined
  onDuplicate?: ((file: WorkspaceFile) => void) | undefined
  renameControls?: RenameControls | undefined
}) {
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
              onDelete={
                visibility === 'private' ? (id) => actions.deleteFile.mutate(id) : undefined
              }
              onRename={onRename}
              onDuplicate={onDuplicate}
              renameControls={renameControls}
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
              onDelete={
                visibility === 'private' ? () => actions.deleteFile.mutate(file.id) : undefined
              }
              onRename={onRename ? () => onRename(file) : undefined}
              onDuplicate={onDuplicate ? () => onDuplicate(file) : undefined}
            >
              <FileTreeFile
                file={file}
                depth={0}
                active={file.id === actions.activeFileId}
                onOpen={actions.open}
                renameControls={renameControls}
              />
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
  onRename,
  onDuplicate,
  renameControls,
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
  onRename?: ((file: WorkspaceFile) => void) | undefined
  onDuplicate?: ((file: WorkspaceFile) => void) | undefined
  renameControls?: RenameControls | undefined
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
  const isRenaming = renameControls?.renamingId === file.id

  const folderRow = isRenaming ? (
    <FileTreeRenameRow
      file={file}
      depth={depth}
      iconName={expanded ? 'folder-open' : 'folder'}
      chevronName={expanded ? 'chevron-down' : 'chevron-right'}
      indentExtra={0}
      renameControls={renameControls}
    />
  ) : (
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
      <span className="min-w-0 flex-1 truncate" title={file.name}>
        {file.name}
      </span>
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
          onRename={onRename ? () => onRename(file) : undefined}
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
              onRename={onRename}
              onDuplicate={onDuplicate}
              renameControls={renameControls}
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
              onRename={onRename ? () => onRename(child) : undefined}
              onDuplicate={onDuplicate ? () => onDuplicate(child) : undefined}
            >
              <FileTreeFile
                file={child}
                depth={depth + 1}
                active={child.id === activeFileId}
                onOpen={onOpenFile}
                renameControls={renameControls}
              />
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
  renameControls,
}: {
  file: WorkspaceFile
  depth: number
  active?: boolean
  onOpen: (file: WorkspaceFile) => void
  renameControls?: RenameControls | undefined
}) {
  if (renameControls?.renamingId === file.id) {
    return (
      <FileTreeRenameRow
        file={file}
        depth={depth}
        iconName={workspaceFileIcon(file)}
        indentExtra={14}
        renameControls={renameControls}
      />
    )
  }

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
      <Icon name={workspaceFileIcon(file)} size={13} className="shrink-0 text-muted-foreground" />
      <span className="min-w-0 flex-1 truncate" title={file.name}>
        {file.name}
      </span>
    </button>
  )
}

/** Shared row shell for inline renaming: keeps the row's icon, indentation,
 *  and compact height, swapping only the name label for an inline Input. */
function FileTreeRenameRow({
  file,
  depth,
  iconName,
  chevronName,
  indentExtra,
  renameControls,
}: {
  file: WorkspaceFile
  depth: number
  iconName: AppIcon
  chevronName?: 'chevron-down' | 'chevron-right' | undefined
  indentExtra: number
  renameControls: RenameControls | undefined
}) {
  if (!renameControls) return null

  return (
    <div
      style={{ paddingLeft: `${6 + depth * 11 + indentExtra}px` }}
      className={cn(
        'mx-1 flex h-6 w-[calc(100%-0.5rem)] min-w-0 items-center rounded-md pr-2',
        chevronName ? 'gap-1.5' : 'gap-2',
      )}
    >
      {chevronName ? (
        <Icon name={chevronName} size={11} className="shrink-0 text-muted-foreground" />
      ) : null}
      <Icon name={iconName} size={13} className="shrink-0 text-muted-foreground" />
      <InlineRenameInput
        file={file}
        pending={renameControls.renamePending}
        fieldError={renameControls.renameError}
        onSubmit={(name) => renameControls.onRenameSubmit(file.id, name)}
        onCancel={renameControls.onRenameCancel}
        onClearError={renameControls.onClearRenameError}
      />
    </div>
  )
}

function InlineRenameInput({
  file,
  pending,
  fieldError,
  onSubmit,
  onCancel,
  onClearError,
}: {
  file: WorkspaceFile
  pending: boolean
  fieldError: string | null
  onSubmit: (name: string) => void
  onCancel: () => void
  onClearError: () => void
}) {
  const [name, setName] = useState(file.name)
  const inputRef = useRef<HTMLInputElement>(null)
  const errorId = useId()

  useEffect(() => {
    const [start, end] =
      file.object_type === 'folder' ? [0, file.name.length] : selectionRange(file.name)
    inputRef.current?.focus()
    inputRef.current?.setSelectionRange(start, end)
    // Only run on mount for this row instance.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (!fieldError || pending) return
    inputRef.current?.focus({ preventScroll: true })
  }, [fieldError, pending])

  const trimmed = name.trim()
  const localError = trimmed === '' ? 'Name is required.' : null
  const displayError = localError ?? fieldError

  function submit() {
    if (pending) return
    if (trimmed === '') {
      queueMicrotask(() => inputRef.current?.focus())
      return
    }
    if (trimmed === file.name) {
      onCancel()
      return
    }
    onSubmit(trimmed)
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    e.stopPropagation()
    if (e.key === 'Enter') {
      e.preventDefault()
      submit()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      onCancel()
    }
  }

  function handleBlur() {
    submit()
  }

  return (
    <div className="relative min-w-0 flex-1">
      <Input
        ref={inputRef}
        value={name}
        disabled={pending}
        aria-label={`${file.object_type === 'folder' ? 'Folder' : 'File'} name`}
        aria-invalid={Boolean(displayError) || undefined}
        aria-describedby={displayError ? errorId : undefined}
        autoComplete="off"
        className="h-5.5 px-1.5 py-0 text-xs"
        onChange={(e) => {
          setName(e.target.value)
          if (fieldError) onClearError()
        }}
        onKeyDown={handleKeyDown}
        onBlur={handleBlur}
        onClick={(e) => e.stopPropagation()}
        onDoubleClick={(e) => e.stopPropagation()}
      />
      {displayError && (
        <span
          id={errorId}
          role="alert"
          className="absolute inset-x-0 top-full z-10 mt-0.5 truncate rounded-md bg-popover px-1.5 py-0.5 text-[10px] text-destructive shadow-sm"
        >
          {displayError}
        </span>
      )}
    </div>
  )
}
