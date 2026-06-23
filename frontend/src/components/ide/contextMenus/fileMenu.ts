import type { ContextMenuItem } from '#/components/ui/context-menu'

export type FileMenuCtx = {
  name: string
  onOpen: () => void
  onOpenToSide: () => void
  onCopyName: () => void
  onSaveAs: () => void
  onDelete?: () => void
}

export function buildFileMenu(ctx: FileMenuCtx): ContextMenuItem[] {
  const items: ContextMenuItem[] = [
    { kind: 'action', id: 'open', label: 'Open', icon: 'folder-open', onSelect: ctx.onOpen },
    { kind: 'action', id: 'open-to-side', label: 'Open to the side', icon: 'column', onSelect: ctx.onOpenToSide },
    { kind: 'separator' },
    { kind: 'action', id: 'copy-name', label: 'Copy name', icon: 'copy-01', onSelect: ctx.onCopyName },
    { kind: 'separator' },
    { kind: 'action', id: 'rename', label: 'Rename', icon: 'pencil-edit-02', soon: true },
    { kind: 'action', id: 'duplicate', label: 'Duplicate', icon: 'copy-01', soon: true },
    { kind: 'action', id: 'save-as', label: 'Save As…', icon: 'download-01', onSelect: ctx.onSaveAs },
  ]
  if (ctx.onDelete) {
    items.push(
      { kind: 'separator' },
      {
        kind: 'action',
        id: 'delete',
        label: 'Delete',
        icon: 'delete-01',
        destructive: true,
        confirm: { title: 'Delete file?', description: `${ctx.name} will be permanently deleted. This action cannot be undone.` },
        onSelect: ctx.onDelete,
      },
    )
  }
  return items
}

export type FolderMenuCtx = {
  name: string
  onCreateFile: () => void
  onCreateFolder: () => void
  onCopyName: () => void
  onDelete?: () => void
}

export function buildFolderMenu(ctx: FolderMenuCtx): ContextMenuItem[] {
  const items: ContextMenuItem[] = [
    { kind: 'action', id: 'new-file', label: 'New file', icon: 'file-01', onSelect: ctx.onCreateFile },
    { kind: 'action', id: 'new-folder', label: 'New folder', icon: 'folder-add', onSelect: ctx.onCreateFolder },
    { kind: 'separator' },
    { kind: 'action', id: 'copy-name', label: 'Copy name', icon: 'copy-01', onSelect: ctx.onCopyName },
    { kind: 'action', id: 'rename', label: 'Rename', icon: 'pencil-edit-02', soon: true },
  ]
  if (ctx.onDelete) {
    items.push(
      { kind: 'separator' },
      {
        kind: 'action',
        id: 'delete',
        label: 'Delete',
        icon: 'delete-01',
        destructive: true,
        confirm: { title: 'Delete folder?', description: `${ctx.name} and all its contents will be permanently deleted. This action cannot be undone.` },
        onSelect: ctx.onDelete,
      },
    )
  }
  return items
}

export type RootMenuCtx = {
  onCreateFile: () => void
  onCreateFolder: () => void
  onRefresh: () => void
}

export function buildRootMenu(ctx: RootMenuCtx): ContextMenuItem[] {
  return [
    { kind: 'action', id: 'new-file', label: 'New file', icon: 'file-01', onSelect: ctx.onCreateFile },
    { kind: 'action', id: 'new-folder', label: 'New folder', icon: 'folder-add', onSelect: ctx.onCreateFolder },
    { kind: 'separator' },
    { kind: 'action', id: 'refresh', label: 'Refresh', icon: 'refresh', onSelect: ctx.onRefresh },
  ]
}
