import { useState } from 'react'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '#/components/ui/dialog'
import { cn } from '#/lib/utils'
import { ReadOnlySqlView } from './object-detail/ReadOnlySqlView'

export type PendingStatementsDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  statements: string[]
}

export function PendingStatementsDialog({
  open,
  onOpenChange,
  statements,
}: PendingStatementsDialogProps) {
  const [selectedStatement, setSelectedStatement] = useState(0)

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (next) setSelectedStatement(0)
        onOpenChange(next)
      }}
    >
      <DialogContent className="flex flex-col sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            Pending statement{statements.length === 1 ? '' : 's'} ({statements.length})
          </DialogTitle>
        </DialogHeader>
        <div className="flex max-h-[60vh] min-h-40 overflow-hidden rounded-md border border-border">
          <div
            role="listbox"
            aria-label="Pending statements"
            className="flex w-52 shrink-0 flex-col overflow-y-auto border-r border-border bg-sidebar"
          >
            {statements.map((statement, index) => {
              const selected = index === selectedStatement
              return (
                <button
                  key={index}
                  type="button"
                  role="option"
                  aria-selected={selected}
                  onClick={() => setSelectedStatement(index)}
                  className={cn(
                    'flex flex-col gap-0.5 border-b border-border px-2.5 py-2 text-left transition-colors hover:bg-accent/40',
                    selected && 'bg-accent',
                  )}
                >
                  <span className="shrink-0 text-[11px] tabular-nums text-muted-foreground">
                    #{index + 1}
                  </span>
                  <span className="min-w-0 truncate font-mono text-[11px] text-foreground">
                    {statement.replace(/\s+/g, ' ').trim()}
                  </span>
                </button>
              )
            })}
          </div>
          <div className="min-w-0 flex-1 overflow-hidden bg-card">
            <ReadOnlySqlView value={statements[selectedStatement] ?? ''} />
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
