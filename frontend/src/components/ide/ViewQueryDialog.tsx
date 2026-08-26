import { Button } from '#/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '#/components/ui/dialog'
import { Icon } from '#/lib/icons'
import { copyWithToast } from './contextMenus/clipboard'

export type ViewQueryDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  sql: string
}

/** Shows the full text of a query that the results caption bar truncates. */
export function ViewQueryDialog({ open, onOpenChange, sql }: ViewQueryDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Query</DialogTitle>
        </DialogHeader>
        <pre className="max-h-96 overflow-auto rounded-md border border-border bg-muted p-2.5 text-xs whitespace-pre-wrap break-words text-foreground">
          {sql}
        </pre>
        <Button variant="outline" size="sm" className="self-end" onClick={() => copyWithToast(sql)}>
          <Icon name="copy-01" size={12} />
          Copy
        </Button>
      </DialogContent>
    </Dialog>
  )
}
