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

type FileNameDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  objectType: 'file' | 'folder'
  currentName: string
  pending: boolean
  fieldError: string | null
  onClearError: () => void
  onSubmit: (name: string) => void
}

/** Selects the "stem" of a filename for the initial highlight: everything
 *  before the last extension, except for dotfiles (`.env`) and names with
 *  no extension (`README`), where the whole name is selected. */
export function selectionRange(name: string): [number, number] {
  const dot = name.lastIndexOf('.')
  if (dot <= 0) return [0, name.length]
  return [0, dot]
}

/** Suggested name for a duplicate: `query.sql` -> `query copy.sql`,
 *  `README` -> `README copy`, `.env` -> `.env copy`. */
export function duplicateFileName(name: string): string {
  const dot = name.lastIndexOf('.')
  if (dot <= 0) return `${name} copy`
  return `${name.slice(0, dot)} copy${name.slice(dot)}`
}

/** Dialog for duplicating a file or folder. Renaming is handled inline in
 *  the file tree row instead (see FilesPanel). */
export function FileNameDialog({
  open,
  onOpenChange,
  objectType,
  currentName,
  pending,
  fieldError,
  onClearError,
  onSubmit,
}: FileNameDialogProps) {
  const [name, setName] = useState(currentName)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!open) return
    const initial = duplicateFileName(currentName)
    setName(initial)
    const timer = setTimeout(() => {
      const [start, end] = selectionRange(initial)
      inputRef.current?.focus()
      inputRef.current?.setSelectionRange(start, end)
    }, 50)
    return () => clearTimeout(timer)
  }, [currentName, open])

  const trimmed = name.trim()
  const disabled = trimmed === '' || pending
  const localError = trimmed === '' ? 'Name is required.' : null
  const displayError = localError ?? fieldError

  function requestClose() {
    if (!pending) onOpenChange(false)
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (disabled) return
    onSubmit(trimmed)
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Escape' && !pending) {
      e.preventDefault()
      requestClose()
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (nextOpen || !pending) onOpenChange(nextOpen)
      }}
    >
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Duplicate {objectType}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <FieldGroup>
            <Field data-invalid={Boolean(displayError)} data-disabled={pending || undefined}>
              <FieldLabel htmlFor="file-name-dialog-input">Name</FieldLabel>
              <Input
                id="file-name-dialog-input"
                ref={inputRef}
                value={name}
                disabled={pending}
                onChange={(e) => {
                  setName(e.target.value)
                  if (fieldError) onClearError()
                }}
                onKeyDown={handleKeyDown}
                autoComplete="off"
                aria-invalid={Boolean(displayError) || undefined}
              />
              {displayError && <FieldError>{displayError}</FieldError>}
            </Field>
          </FieldGroup>
          <DialogFooter className="mt-4">
            <Button type="button" variant="outline" disabled={pending} onClick={requestClose}>
              Cancel
            </Button>
            <Button type="submit" disabled={disabled}>
              {pending ? 'Duplicating…' : 'Duplicate'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
