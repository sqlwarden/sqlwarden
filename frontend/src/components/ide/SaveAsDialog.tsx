import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { HugeiconsIcon } from '@hugeicons/react'
import { FolderIcon, FolderOpenIcon } from '@hugeicons/core-free-icons'
import { Button } from '#/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '#/components/ui/dialog'
import { Input } from '#/components/ui/input'
import { Label } from '#/components/ui/label'
import { ScrollArea } from '#/components/ui/scroll-area'
import { createPrivateWorkspaceFile, updatePrivateWorkspaceFileContent } from '#/lib/api/files'
import { orgWorkspacePrivateFileBrowserQueryOptions } from '#/lib/api/query'
import type { WorkspaceFile } from '#/lib/api/types'
import { ApiError } from '#/lib/api/errors'
import { cn } from '#/lib/utils'
import type { EditorTab } from './useIdeStore'

type SaveAsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  tab: EditorTab
  orgSlug: string
  workspaceId: number
  onSuccess: (file: WorkspaceFile, etag: string) => void
}

export function SaveAsDialog({
  open,
  onOpenChange,
  tab,
  orgSlug,
  workspaceId,
  onSuccess,
}: SaveAsDialogProps) {
  const [name, setName] = useState('untitled.sql')
  const [selectedParentId, setSelectedParentId] = useState<number | null>(null)
  const [fieldError, setFieldError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const queryClient = useQueryClient()

  useEffect(() => {
    if (open) {
      setName('untitled.sql')
      setSelectedParentId(null)
      setFieldError(null)
      setTimeout(() => inputRef.current?.select(), 50)
    }
  }, [open])

  const rootBrowser = useQuery(
    orgWorkspacePrivateFileBrowserQueryOptions(orgSlug, workspaceId, null),
  )
  const folders = (rootBrowser.data?.children ?? []).filter(
    (f) => f.object_type === 'folder',
  )

  const save = useMutation({
    mutationFn: async () => {
      const file = await createPrivateWorkspaceFile(orgSlug, workspaceId, {
        name: name.trim(),
        object_type: 'file',
        parent_id: selectedParentId,
        media_type: 'text/plain',
        file_kind: 'sql',
      })
      const result = await updatePrivateWorkspaceFileContent(
        orgSlug,
        workspaceId,
        file.id,
        tab.content,
      )
      return { file, etag: result.etag }
    },
    onSuccess: ({ file, etag }) => {
      queryClient.invalidateQueries({
        queryKey: orgWorkspacePrivateFileBrowserQueryOptions(orgSlug, workspaceId, selectedParentId).queryKey,
      })
      onSuccess(file, etag)
      onOpenChange(false)
    },
    onError: (err) => {
      if (err instanceof ApiError && err.fieldErrors?.name) {
        setFieldError(err.fieldErrors.name)
      } else {
        setFieldError('Something went wrong. Please try again.')
      }
    },
  })

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    setFieldError(null)
    save.mutate()
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Save As</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="save-as-name">Name</Label>
            <Input
              id="save-as-name"
              ref={inputRef}
              value={name}
              onChange={(e) => { setName(e.target.value); setFieldError(null) }}
              placeholder="untitled.sql"
              autoComplete="off"
              aria-invalid={!!fieldError}
            />
            {fieldError && <p className="text-xs text-destructive">{fieldError}</p>}
          </div>

          <div className="flex flex-col gap-1.5">
            <Label>Location</Label>
            <ScrollArea className="h-36 rounded-md border border-border">
              <div className="flex flex-col p-1">
                <LocationRow
                  label="My Files (root)"
                  icon={FolderOpenIcon}
                  selected={selectedParentId === null}
                  onSelect={() => setSelectedParentId(null)}
                  indent={0}
                />
                {folders.map((folder) => (
                  <LocationRow
                    key={folder.id}
                    label={folder.name}
                    icon={FolderIcon}
                    selected={selectedParentId === folder.id}
                    onSelect={() => setSelectedParentId(folder.id)}
                    indent={1}
                  />
                ))}
              </div>
            </ScrollArea>
          </div>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!name.trim() || save.isPending}>
              {save.isPending ? 'Saving…' : 'Save'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function LocationRow({
  label,
  icon,
  selected,
  onSelect,
  indent,
}: {
  label: string
  icon: typeof FolderIcon
  selected: boolean
  onSelect: () => void
  indent: number
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      style={{ paddingLeft: `${8 + indent * 12}px` }}
      className={cn(
        'flex h-7 w-full items-center gap-2 rounded pr-2 text-left text-xs transition-colors',
        selected
          ? 'bg-accent text-accent-foreground'
          : 'hover:bg-accent/50 hover:text-accent-foreground',
      )}
    >
      <HugeiconsIcon icon={icon} size={13} strokeWidth={2} className="shrink-0 text-muted-foreground" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {selected && <span className="shrink-0 text-[10px] text-muted-foreground">✓</span>}
    </button>
  )
}
