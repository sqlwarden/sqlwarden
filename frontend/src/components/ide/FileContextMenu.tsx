import { useState } from 'react'
import { HugeiconsIcon } from '@hugeicons/react'
import { Delete01Icon, File01Icon, FolderAddIcon, FolderOpenIcon } from '@hugeicons/core-free-icons'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '#/components/ui/alert-dialog'

export type FileContextMenuKind = 'root' | 'folder' | 'file'

type FileContextMenuProps = {
  children: React.ReactNode
  kind: FileContextMenuKind
  nodeId?: number
  nodeName?: string
  onOpen?: () => void
  onCreateFile?: (parentId: number | null) => void
  onCreateFolder?: (parentId: number | null) => void
  onDelete?: () => void
}

export function FileContextMenu({
  children,
  kind,
  nodeId,
  nodeName,
  onOpen,
  onCreateFile,
  onCreateFolder,
  onDelete,
}: FileContextMenuProps) {
  const [menuOpen, setMenuOpen] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [pos, setPos] = useState({ x: 0, y: 0 })

  function handleContextMenu(e: React.MouseEvent) {
    e.preventDefault()
    e.stopPropagation()
    setPos({ x: e.clientX, y: e.clientY })
    setMenuOpen(true)
  }

  const parentId = kind === 'root' ? null : (nodeId ?? null)
  const isFolder = kind === 'folder'
  const canDelete = (kind === 'file' || kind === 'folder') && !!onDelete

  return (
    <>
      <div onContextMenu={handleContextMenu} className={menuOpen ? 'bg-accent text-accent-foreground' : ''}>
        {children}
        <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
          <DropdownMenuTrigger
            nativeButton={false}
            render={
              <span
                style={{
                  position: 'fixed',
                  left: pos.x,
                  top: pos.y,
                  width: 0,
                  height: 0,
                  pointerEvents: 'none',
                }}
              />
            }
          />
          <DropdownMenuContent align="start" side="bottom" sideOffset={2} className="w-44">
            {(kind === 'root' || kind === 'folder') && (
              <>
                <DropdownMenuItem
                  onClick={() => { setMenuOpen(false); onCreateFile?.(parentId) }}
                >
                  <HugeiconsIcon icon={File01Icon} size={13} strokeWidth={2} />
                  New File
                </DropdownMenuItem>
                <DropdownMenuItem
                  onClick={() => { setMenuOpen(false); onCreateFolder?.(parentId) }}
                >
                  <HugeiconsIcon icon={FolderAddIcon} size={13} strokeWidth={2} />
                  New Folder
                </DropdownMenuItem>
              </>
            )}
            {kind === 'file' && (
              <DropdownMenuItem
                onClick={() => { setMenuOpen(false); onOpen?.() }}
              >
                <HugeiconsIcon icon={FolderOpenIcon} size={13} strokeWidth={2} />
                Open
              </DropdownMenuItem>
            )}
            {canDelete && (
              <DropdownMenuItem
                data-variant="destructive"
                onClick={() => { setMenuOpen(false); setConfirmOpen(true) }}
              >
                <HugeiconsIcon icon={Delete01Icon} size={13} strokeWidth={2} />
                Delete
              </DropdownMenuItem>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Delete {isFolder ? 'folder' : 'file'}?
            </AlertDialogTitle>
            <AlertDialogDescription>
              {nodeName ? (
                <>
                  <span className="font-medium text-foreground">{nodeName}</span>
                  {isFolder
                    ? ' and all its contents will be permanently deleted.'
                    : ' will be permanently deleted.'}
                </>
              ) : (
                'This will be permanently deleted.'
              )}
              {' '}This action cannot be undone.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => { setConfirmOpen(false); onDelete?.() }}
              className="bg-destructive text-white hover:bg-destructive/80"
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  )
}
