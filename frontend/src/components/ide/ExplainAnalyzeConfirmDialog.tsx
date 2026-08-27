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

export type ExplainAnalyzeConfirmDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  sql: string
  onConfirm: () => void
}

/** Confirms EXPLAIN ANALYZE before running it — unlike a plain EXPLAIN, this
 *  executes the statement for real to capture runtime stats, regardless of
 *  the statement's classified kind. Cancel leaves the pending state
 *  untouched by the caller, Explain Analyze resubmits the confirmed run. */
export function ExplainAnalyzeConfirmDialog({
  open,
  onOpenChange,
  sql,
  onConfirm,
}: ExplainAnalyzeConfirmDialogProps) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Run EXPLAIN ANALYZE?</AlertDialogTitle>
          <AlertDialogDescription>
            EXPLAIN ANALYZE executes the statement for real to capture runtime stats, even if it
            changes data.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <pre className="max-h-40 overflow-auto rounded-md border border-border bg-muted p-2.5 font-mono text-xs text-foreground">
          {sql}
        </pre>
        <AlertDialogFooter>
          <AlertDialogCancel variant="ghost">Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onConfirm}>
            Explain Analyze
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
