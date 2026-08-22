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

export type TransactionGuardReason = 'switch-to-auto' | 'close-connection'

const REASON_COPY: Record<TransactionGuardReason, { title: string; description: string }> = {
  'switch-to-auto': {
    title: 'Commit or roll back before switching to auto-commit?',
    description:
      'This connection has an open transaction. Switching to auto-commit requires resolving it first.',
  },
  'close-connection': {
    title: 'Commit or roll back before closing?',
    description:
      'This connection has an open transaction. Closing without resolving it will roll back every uncommitted change.',
  },
}

export type TransactionGuardDialogProps = {
  open: boolean
  reason: TransactionGuardReason
  pendingStatements: number
  onCommit: () => void
  onRollback: () => void
  onOpenChange: (open: boolean) => void
}

/** Blocks switching Manual→Auto or closing the last tab/disconnecting while
 *  a transaction is open, mirroring UnsafeQueryDialog's Cancel/confirm shape
 *  but with two destructive actions (Commit, Rollback) instead of one. */
export function TransactionGuardDialog({
  open,
  reason,
  pendingStatements,
  onCommit,
  onRollback,
  onOpenChange,
}: TransactionGuardDialogProps) {
  const copy = REASON_COPY[reason]
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{copy.title}</AlertDialogTitle>
          <AlertDialogDescription>
            {copy.description} {pendingStatements} statement{pendingStatements === 1 ? '' : 's'}{' '}
            pending.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel variant="ghost">Cancel</AlertDialogCancel>
          <AlertDialogAction variant="outline" onClick={onRollback}>
            Rollback
          </AlertDialogAction>
          <AlertDialogAction variant="default" onClick={onCommit}>
            Commit
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
