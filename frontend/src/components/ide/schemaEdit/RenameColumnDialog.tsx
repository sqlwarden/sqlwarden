import { useEffect, useRef, useState } from 'react'
import { Button } from '#/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '#/components/ui/dialog'
import { Field, FieldError, FieldGroup, FieldLabel } from '#/components/ui/field'
import { Input } from '#/components/ui/input'

export type RenameColumnDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  tableName: string
  columnName: string
  pending: boolean
  onSubmit: (newName: string) => void
}

/** Compact, focused dialog for renaming a single table column. */
export function RenameColumnDialog({
  open,
  onOpenChange,
  tableName,
  columnName,
  pending,
  onSubmit,
}: RenameColumnDialogProps) {
  const [name, setName] = useState(columnName)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!open) return
    setName(columnName)
    const timer = setTimeout(() => {
      inputRef.current?.focus()
      inputRef.current?.select()
    }, 50)
    return () => clearTimeout(timer)
  }, [open, columnName])

  const trimmed = name.trim()
  const error =
    trimmed === ''
      ? 'Column name is required.'
      : trimmed === columnName
        ? 'Enter a different name.'
        : null
  const disabled = pending || Boolean(error)

  function requestClose() {
    if (!pending) onOpenChange(false)
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (disabled) return
    onSubmit(trimmed)
  }

  return (
    <Dialog open={open} onOpenChange={(next) => (next || !pending) && onOpenChange(next)}>
      <DialogContent className="sm:max-w-xs">
        <DialogHeader>
          <DialogTitle>Rename column</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <FieldGroup>
            <Field data-invalid={name.trim() !== '' && Boolean(error)}>
              <FieldLabel htmlFor="rename-column-input">
                New name for {tableName}.{columnName}
              </FieldLabel>
              <Input
                id="rename-column-input"
                ref={inputRef}
                value={name}
                disabled={pending}
                onChange={(e) => setName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Escape' && !pending) {
                    e.preventDefault()
                    requestClose()
                  }
                }}
                autoComplete="off"
                aria-invalid={Boolean(error) || undefined}
              />
              {name.trim() !== '' && error && <FieldError>{error}</FieldError>}
            </Field>
          </FieldGroup>
          <DialogFooter className="mt-4">
            <Button type="button" variant="outline" disabled={pending} onClick={requestClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={disabled}>
              {pending ? 'Renaming…' : 'Rename'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
