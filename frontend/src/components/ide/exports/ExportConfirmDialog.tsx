import { Button } from '#/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '#/components/ui/dialog'
import { Icon } from '#/lib/icons'
import { countSqlStatements } from '../sqlStatements'

type ExportConfirmDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  sql: string
  onConfirm: () => void
}

/** Confirms the resolved query before a direct download, since the toolbar's Run selection can change between clicks. */
export function ExportConfirmDialog({
  open,
  onOpenChange,
  sql,
  onConfirm,
}: ExportConfirmDialogProps) {
  const isMultiStatement = countSqlStatements(sql) > 1

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Export query results</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-2">
          <p className="text-xs text-muted-foreground">
            This query will run and the results will export as a file.
          </p>
          <pre className="max-h-40 overflow-auto rounded-md border border-border bg-muted/40 p-2 font-mono text-[11px] whitespace-pre-wrap text-foreground">
            {sql}
          </pre>
          {isMultiStatement && (
            <p className="rounded-md border border-destructive/30 bg-destructive/10 px-2 py-1.5 text-[11px] text-destructive">
              Multiple queries were selected. Only a single query can be exported right now — select
              just the query you want.
            </p>
          )}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button type="button" disabled={isMultiStatement} onClick={onConfirm}>
            <Icon name="download-01" size={13} data-icon="inline-start" />
            Export
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
