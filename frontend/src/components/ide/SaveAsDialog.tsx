import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Icon } from '#/lib/icons'
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
import { cn } from '#/lib/utils'
import type { EditorTab } from './useIdeStore'
import { FILE_TYPES, DEFAULT_FILE_TYPE, buildFilename, type FileType } from './fileTypes'

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
  const [basename, setBasename] = useState('untitled')
  const [fileType, setFileType] = useState<FileType>(DEFAULT_FILE_TYPE)
  const [selectedParentId, setSelectedParentId] = useState<number | null>(null)
  const [fieldError, setFieldError] = useState<string | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const queryClient = useQueryClient()

  useEffect(() => {
    if (open) {
      setBasename('untitled')
      setFileType(DEFAULT_FILE_TYPE)
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

  const filename = buildFilename(basename, fileType)

  const save = useMutation({
    mutationFn: async () => {
      const file = await createPrivateWorkspaceFile(orgSlug, workspaceId, {
        name: filename,
        object_type: 'file',
        parent_id: selectedParentId,
        media_type: fileType.mediaType,
        file_kind: fileType.kind,
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
    if (!basename.trim()) return
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

          {/* Name + type row */}
          <div className="flex flex-col gap-1.5">
            <Label htmlFor="save-as-name">Name</Label>
            <div className="flex gap-2">
              <Input
                id="save-as-name"
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
                Will save as <span className="font-mono text-foreground">{filename}</span>
              </p>
            )}
          </div>

          {/* Location picker */}
          <div className="flex flex-col gap-1.5">
            <Label>Location</Label>
            <ScrollArea className="h-36 rounded-md border border-border">
              <div className="flex flex-col p-1">
                <LocationRow
                  label="My Files (root)"
                  icon="folder-open"
                  selected={selectedParentId === null}
                  onSelect={() => setSelectedParentId(null)}
                  indent={0}
                />
                {folders.map((folder) => (
                  <LocationRow
                    key={folder.id}
                    label={folder.name}
                    icon="folder"
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
            <Button type="submit" disabled={!basename.trim() || save.isPending}>
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
  icon: import('#/lib/icons').AppIcon
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
      <Icon name={icon} size={13} className="shrink-0 text-muted-foreground" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {selected && <span className="shrink-0 text-[10px] text-muted-foreground">✓</span>}
    </button>
  )
}
