import { useState } from 'react'
import { HugeiconsIcon } from '@hugeicons/react'
import { File01Icon, FolderAddIcon, FolderOpenIcon } from '@hugeicons/core-free-icons'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'

export type FileContextMenuKind = 'root' | 'folder' | 'file'

type FileContextMenuProps = {
  children: React.ReactNode
  kind: FileContextMenuKind
  nodeId?: number
  onOpen?: () => void
  onCreateFile?: (parentId: number | null) => void
  onCreateFolder?: (parentId: number | null) => void
}

export function FileContextMenu({
  children,
  kind,
  nodeId,
  onOpen,
  onCreateFile,
  onCreateFolder,
}: FileContextMenuProps) {
  const [open, setOpen] = useState(false)
  const [pos, setPos] = useState({ x: 0, y: 0 })

  function handleContextMenu(e: React.MouseEvent) {
    e.preventDefault()
    e.stopPropagation()
    setPos({ x: e.clientX, y: e.clientY })
    setOpen(true)
  }

  const parentId = kind === 'root' ? null : (nodeId ?? null)

  return (
    <div onContextMenu={handleContextMenu} className="contents">
      {children}
      <DropdownMenu open={open} onOpenChange={setOpen}>
        <DropdownMenuTrigger
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
                onClick={() => {
                  setOpen(false)
                  onCreateFile?.(parentId)
                }}
              >
                <HugeiconsIcon icon={File01Icon} size={13} strokeWidth={2} />
                New File
              </DropdownMenuItem>
              <DropdownMenuItem
                onClick={() => {
                  setOpen(false)
                  onCreateFolder?.(parentId)
                }}
              >
                <HugeiconsIcon icon={FolderAddIcon} size={13} strokeWidth={2} />
                New Folder
              </DropdownMenuItem>
            </>
          )}
          {kind === 'file' && (
            <DropdownMenuItem
              onClick={() => {
                setOpen(false)
                onOpen?.()
              }}
            >
              <HugeiconsIcon icon={FolderOpenIcon} size={13} strokeWidth={2} />
              Open
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
