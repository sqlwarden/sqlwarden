import { ContextMenu } from '#/components/ui/context-menu'
import { copyWithToast } from './contextMenus/clipboard'
import { buildFileMenu, buildFolderMenu, buildRootMenu } from './contextMenus/fileMenu'

export type FileContextMenuKind = 'root' | 'folder' | 'file'

type FileContextMenuProps = {
  children: React.ReactNode
  kind: FileContextMenuKind
  nodeId?: number
  nodeName?: string
  className?: string
  onOpen?: () => void
  onOpenToSide?: () => void
  onSaveAs?: () => void
  onCreateFile?: (parentId: number | null) => void
  onCreateFolder?: (parentId: number | null) => void
  onDelete?: () => void
  onRefresh?: () => void
}

export function FileContextMenu({
  children,
  kind,
  nodeId,
  nodeName,
  className,
  onOpen,
  onOpenToSide,
  onSaveAs,
  onCreateFile,
  onCreateFolder,
  onDelete,
  onRefresh,
}: FileContextMenuProps) {
  const parentId = kind === 'root' ? null : (nodeId ?? null)
  const name = nodeName ?? ''

  const items =
    kind === 'file'
      ? buildFileMenu({
          name,
          onOpen: () => onOpen?.(),
          onOpenToSide: () => onOpenToSide?.(),
          onCopyName: () => copyWithToast(name),
          onSaveAs: () => onSaveAs?.(),
          onDelete,
        })
      : kind === 'folder'
        ? buildFolderMenu({
            name,
            onCreateFile: () => onCreateFile?.(parentId),
            onCreateFolder: () => onCreateFolder?.(parentId),
            onCopyName: () => copyWithToast(name),
            onDelete,
          })
        : buildRootMenu({
            onCreateFile: () => onCreateFile?.(parentId),
            onCreateFolder: () => onCreateFolder?.(parentId),
            onRefresh: () => onRefresh?.(),
          })

  return (
    <ContextMenu items={items} className={className}>
      {children}
    </ContextMenu>
  )
}
