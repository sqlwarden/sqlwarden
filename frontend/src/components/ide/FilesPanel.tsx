import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { HugeiconsIcon } from '@hugeicons/react'
import {
  ArrowDown01Icon,
  ArrowRight01Icon,
  File01Icon,
  FolderIcon,
  FolderOpenIcon,
} from '@hugeicons/core-free-icons'
import { Separator } from '#/components/ui/separator'
import {
  orgWorkspacePrivateFileBrowserQueryOptions,
  orgWorkspaceSharedFileBrowserQueryOptions,
} from '#/lib/api/query'
import type { Workspace, WorkspaceFile } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { useIde, newFileTab } from './useIdeStore'
import { SidebarPane } from './SidebarPane'

type FilesPanelProps = {
  orgSlug: string
  workspace: Workspace
  maximized: boolean
  onMaximizedChange: (maximized: boolean) => void
}

export function FilesPanel({ orgSlug, workspace, maximized, onMaximizedChange }: FilesPanelProps) {
  return (
    <SidebarPane
      title="Files"
      icon={FolderOpenIcon}
      maximized={maximized}
      onMaximizedChange={onMaximizedChange}
    >
      <FilesSection
        orgSlug={orgSlug}
        workspace={workspace}
        visibility="private"
        title="My Files"
      />
      <Separator className="my-1" />
      <FilesSection
        orgSlug={orgSlug}
        workspace={workspace}
        visibility="shared"
        title="Shared Files"
      />
    </SidebarPane>
  )
}

function FilesSection({
  orgSlug,
  workspace,
  visibility,
  title,
}: {
  orgSlug: string
  workspace: Workspace
  visibility: 'private' | 'shared'
  title: string
}) {
  const openTab = useIde((s) => s.openTab)

  const queryOptions =
    visibility === 'private'
      ? orgWorkspacePrivateFileBrowserQueryOptions(orgSlug, workspace.id, null)
      : orgWorkspaceSharedFileBrowserQueryOptions(orgSlug, workspace.id, null)

  const { data, isLoading, isError } = useQuery(queryOptions)

  function handleOpenFile(file: WorkspaceFile) {
    openTab(newFileTab(file, workspace))
  }

  const children = data?.children ?? []

  return (
    <div className="flex flex-col">
      <div className="px-2 py-1.5 text-[10px] font-semibold uppercase tracking-widest text-muted-foreground">
        {title}
      </div>
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
            />
          ) : (
            <FileTreeFile key={file.id} file={file} depth={0} onOpen={handleOpenFile} />
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
}: {
  file: WorkspaceFile
  orgSlug: string
  workspaceId: number
  visibility: 'private' | 'shared'
  depth: number
  onOpenFile: (file: WorkspaceFile) => void
}) {
  const [expanded, setExpanded] = useState(false)

  const queryOptions =
    visibility === 'private'
      ? orgWorkspacePrivateFileBrowserQueryOptions(orgSlug, workspaceId, file.id)
      : orgWorkspaceSharedFileBrowserQueryOptions(orgSlug, workspaceId, file.id)

  const { data } = useQuery({ ...queryOptions, enabled: expanded })

  const children = data?.children ?? []

  return (
    <>
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        style={{ paddingLeft: `${8 + depth * 12}px` }}
        className={cn(
          'flex h-7 w-full items-center gap-1.5 pr-2 text-left text-xs',
          'transition-colors hover:bg-accent hover:text-accent-foreground',
        )}
      >
        <HugeiconsIcon
          icon={expanded ? ArrowDown01Icon : ArrowRight01Icon}
          size={11}
          strokeWidth={2}
          className="shrink-0 text-muted-foreground"
        />
        <HugeiconsIcon
          icon={expanded ? FolderOpenIcon : FolderIcon}
          size={14}
          strokeWidth={2}
          className="shrink-0 text-muted-foreground"
        />
        <span className="min-w-0 flex-1 truncate">{file.name}</span>
      </button>

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
            />
          ) : (
            <FileTreeFile key={child.id} file={child} depth={depth + 1} onOpen={onOpenFile} />
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
      <HugeiconsIcon icon={File01Icon} size={13} strokeWidth={2} className="shrink-0 text-muted-foreground" />
      <span className="min-w-0 flex-1 truncate">{file.name}</span>
    </button>
  )
}
