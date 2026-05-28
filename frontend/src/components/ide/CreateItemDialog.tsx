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
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '#/components/ui/select'
import { createPrivateWorkspaceFile, updatePrivateWorkspaceFileContent } from '#/lib/api/files'
import { orgWorkspacePrivateFileBrowserQueryOptions } from '#/lib/api/query'
import type { WorkspaceFile } from '#/lib/api/types'
import { ApiError } from '#/lib/api/errors'
import { FILE_TYPES, DEFAULT_FILE_TYPE, buildFilename, type FileType } from './fileTypes'

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
  const [basename, setBasename] = useState(kind === 'file' ? 'untitled' : '')
  const [fileType, setFileType] = useState<FileType>(DEFAULT_FILE_TYPE)
  const [fieldError, setFieldError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const queryClient = useQueryClient()

  useEffect(() => {
    if (open) {
      setBasename(kind === 'file' ? 'untitled' : '')
      setFileType(DEFAULT_FILE_TYPE)
      setFieldError(null)
      setTimeout(() => {
        inputRef.current?.select()
      }, 50)
    }
  }, [open, kind])

  const filename = kind === 'file' ? buildFilename(basename, fileType) : basename.trim()

  const create = useMutation({
    mutationFn: async () => {
      const file = await createPrivateWorkspaceFile(orgSlug, workspaceId, {
        name: filename,
        object_type: kind,
        parent_id: parentId,
        ...(kind === 'file'
          ? { media_type: fileType.mediaType, file_kind: fileType.kind }
          : {}),
      })
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
    if (!basename.trim()) return
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
            {kind === 'file' ? (
              <>
                <div className="flex gap-2">
                  <Input
                    id="create-item-name"
                    ref={inputRef}
                    value={basename}
                    onChange={(e) => { setBasename(e.target.value); setFieldError(null) }}
                    placeholder="untitled"
                    autoComplete="off"
                    aria-invalid={!!fieldError}
                    className="min-w-0 flex-1"
                  />
                  <Select
                    items={FILE_TYPES.map((t) => ({ label: `${t.label} (${t.extension})`, value: t.kind }))}
                    value={fileType.kind}
                    onValueChange={(kind) => {
                      const t = FILE_TYPES.find((ft) => ft.kind === kind)
                      if (t) setFileType(t)
                    }}
                  >
                    <SelectTrigger className="w-32 shrink-0">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        {FILE_TYPES.map((t) => (
                          <SelectItem key={t.kind} value={t.kind}>
                            {t.label} ({t.extension})
                          </SelectItem>
                        ))}
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </div>
                {fieldError ? (
                  <p className="text-xs text-destructive">{fieldError}</p>
                ) : (
                  <p className="text-xs text-muted-foreground">
                    Will create <span className="font-mono text-foreground">{filename}</span>
                  </p>
                )}
              </>
            ) : (
              <>
                <Input
                  id="create-item-name"
                  ref={inputRef}
                  value={basename}
                  onChange={(e) => { setBasename(e.target.value); setFieldError(null) }}
                  placeholder="folder name"
                  autoComplete="off"
                  aria-invalid={!!fieldError}
                />
                {fieldError && (
                  <p className="text-xs text-destructive">{fieldError}</p>
                )}
              </>
            )}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={!basename.trim() || create.isPending}>
              {create.isPending ? 'Creating…' : 'Create'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
