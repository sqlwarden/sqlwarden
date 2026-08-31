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
import { ReadOnlySqlView } from './object-detail/ReadOnlySqlView'

export type UnsafeQueryDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  sql: string
  onConfirm: () => void
}

/** Confirms an UPDATE/DELETE the backend flagged as unsafe (no WHERE clause)
 *  before letting it run — Cancel leaves the pending state untouched by the
 *  caller, Run Anyway resubmits with confirmUnsafe: true. */
export function UnsafeQueryDialog({ open, onOpenChange, sql, onConfirm }: UnsafeQueryDialogProps) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Run without a WHERE clause?</AlertDialogTitle>
          <AlertDialogDescription>
            This statement has no WHERE clause and will affect every row in the table.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="h-40 overflow-hidden rounded-md border border-border bg-muted">
          <ReadOnlySqlView value={sql} />
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel variant="ghost">Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onConfirm}>
            Run Anyway
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
