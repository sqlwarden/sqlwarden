import { useEffect, useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
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
import { createPrivateWorkspaceFile, updatePrivateWorkspaceFileContent } from '#/lib/api/files'
import { orgWorkspacePrivateFileBrowserQueryOptions } from '#/lib/api/query'
import type { WorkspaceFile } from '#/lib/api/types'
import { ApiError } from '#/lib/api/errors'

type CreateItemDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  kind: 'file' | 'folder'
  parentId: number | null
  orgSlug: string
  workspaceId: number
  onSuccess: (file: WorkspaceFile) => void
}

export function CreateItemDialog({
  open,
  onOpenChange,
  kind,
  parentId,
  orgSlug,
  workspaceId,
  onSuccess,
}: CreateItemDialogProps) {
  const [name, setName] = useState(kind === 'file' ? 'untitled.sql' : '')
  const [fieldError, setFieldError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const queryClient = useQueryClient()

  useEffect(() => {
    if (open) {
      setName(kind === 'file' ? 'untitled.sql' : '')
      setFieldError(null)
      setTimeout(() => inputRef.current?.select(), 50)
    }
  }, [open, kind])

  const create = useMutation({
    mutationFn: async () => {
      const file = await createPrivateWorkspaceFile(orgSlug, workspaceId, {
        name: name.trim(),
        object_type: kind,
        parent_id: parentId,
        ...(kind === 'file' ? { media_type: 'text/plain', file_kind: 'sql' } : {}),
      })
      // Write empty content immediately so the file is readable on first open.
      // Folders don't have content.
      if (kind === 'file') {
        await updatePrivateWorkspaceFileContent(orgSlug, workspaceId, file.id, '')
      }
      return file
    },
    onSuccess: (file) => {
      queryClient.invalidateQueries({
        queryKey: orgWorkspacePrivateFileBrowserQueryOptions(orgSlug, workspaceId, parentId).queryKey,
      })
      onSuccess(file)
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
    create.mutate()
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{kind === 'file' ? 'New File' : 'New Folder'}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="create-item-name">Name</Label>
            <Input
              id="create-item-name"
              ref={inputRef}
              value={name}
              onChange={(e) => { setName(e.target.value); setFieldError(null) }}
              placeholder={kind === 'file' ? 'untitled.sql' : 'folder name'}
              autoComplete="off"
              aria-invalid={!!fieldError}
            />
            {fieldError && (
              <p className="text-xs text-destructive">{fieldError}</p>
            )}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!name.trim() || create.isPending}>
              {create.isPending ? 'Creating…' : 'Create'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
