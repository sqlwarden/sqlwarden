import { useEffect, useRef, useState } from 'react'
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
import { useFavoritesMutations } from './useFavoritesMutations'

type SaveFavoriteDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgSlug: string
  workspaceId: number
  sqlText: string
  connectionId: number | null
  onSaved?: () => void
}

export function SaveFavoriteDialog({
  open,
  onOpenChange,
  orgSlug,
  workspaceId,
  sqlText,
  connectionId,
  onSaved,
}: SaveFavoriteDialogProps) {
  const mutations = useFavoritesMutations(orgSlug, workspaceId)
  const [name, setName] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (open) {
      setName('')
      setTimeout(() => inputRef.current?.focus(), 50)
    }
  }, [open])

  async function handleSave() {
    if (!name.trim()) return
    setIsSaving(true)
    try {
      await mutations.create({ name: name.trim(), sqlText, connectionId })
      onOpenChange(false)
      onSaved?.()
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Save as favorite</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-1.5">
          <Label htmlFor="favorite-name">Name</Label>
          <Input
            id="favorite-name"
            ref={inputRef}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Name this query"
            autoComplete="off"
          />
        </div>
        <div className="flex flex-col gap-1.5">
          <Label>Query</Label>
          <div className="max-h-32 overflow-y-auto rounded-md border border-input bg-muted/40 px-2 py-1.5">
            <code className="block whitespace-pre-wrap break-words font-mono text-xs leading-snug text-foreground">
              {sqlText}
            </code>
          </div>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            type="button"
            disabled={!name.trim() || isSaving}
            onClick={() => void handleSave()}
          >
            {isSaving ? 'Saving…' : 'Save'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
