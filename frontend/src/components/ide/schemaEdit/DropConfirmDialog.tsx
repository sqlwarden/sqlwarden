import { useEffect, useState } from 'react'
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
import { Checkbox } from '#/components/ui/checkbox'

export type DropConfirmDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** e.g. "table", "schema", "column", "index" — used for the confirm label. */
  objectType: string
  objectName: string
  description: React.ReactNode
  cascadeAvailable: boolean
  pending: boolean
  onConfirm: (cascade: boolean) => void
}

/** Shared destructive confirmation for every drop operation (object, scope,
 *  column, index), mirroring the app's existing delete-connection pattern. */
export function DropConfirmDialog({
  open,
  onOpenChange,
  objectType,
  objectName,
  description,
  cascadeAvailable,
  pending,
  onConfirm,
}: DropConfirmDialogProps) {
  const [cascade, setCascade] = useState(false)

  useEffect(() => {
    if (open) setCascade(false)
  }, [open])

  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (next || !pending) onOpenChange(next)
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            Drop {objectType} <span className="font-mono">{objectName}</span>?
          </AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        {cascadeAvailable && (
          <label className="flex cursor-pointer items-start gap-2.5 rounded-md border border-border p-3 text-xs">
            <Checkbox
              checked={cascade}
              disabled={pending}
              onCheckedChange={(checked) => setCascade(checked === true)}
            />
            <span className="flex flex-col gap-0.5">
              <span className="font-medium text-foreground">Cascade</span>
              <span className="text-muted-foreground">
                Also drop objects that depend on this {objectType}.
              </span>
            </span>
          </label>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel variant="ghost" disabled={pending}>
            Cancel
          </AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={pending}
            onClick={() => onConfirm(cascade)}
          >
            {pending ? 'Dropping…' : 'Drop'}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
