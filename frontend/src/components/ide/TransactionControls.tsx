import { useState } from 'react'
import { Icon } from '#/lib/icons'
import { Button } from '#/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '#/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '#/components/ui/dropdown-menu'
import type { TransactionMode } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { getFrontendEngine } from './engines/registry'
import { ReadOnlySqlView } from './object-detail/ReadOnlySqlView'
import { Tip } from './schema-diagram/Tip'
import type { TransactionState } from './useIdeStore'

export type TransactionControlsProps = {
  state: TransactionState
  driver: string
  switchToManual: () => void
  switchToAuto: () => Promise<'ok' | 'blocked'>
  commit: () => Promise<void>
  rollback: () => Promise<void>
  /** Called instead of switching when a transaction is open — the caller
   *  shows the Commit/Rollback/Cancel guard dialog. */
  onSwitchToAutoBlocked: () => void
}

export function TransactionControls({
  state,
  driver,
  switchToManual,
  switchToAuto,
  commit,
  rollback,
  onSwitchToAutoBlocked,
}: TransactionControlsProps) {
  const manualTransactionWarning = getFrontendEngine(driver).manualTransactionWarning
  const [detailsOpen, setDetailsOpen] = useState(false)
  const [selectedStatement, setSelectedStatement] = useState(0)

  function openDetails() {
    setSelectedStatement(0)
    setDetailsOpen(true)
  }

  async function selectMode(mode: TransactionMode) {
    if (mode === state.mode) return
    if (mode === 'manual') {
      switchToManual()
      return
    }
    const result = await switchToAuto()
    if (result === 'blocked') onSwitchToAutoBlocked()
  }

  return (
    <div className="flex items-center gap-2">
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-7 gap-1.5 px-2 text-xs font-normal"
            />
          }
        >
          <span
            className={cn(
              'size-1.5 shrink-0 rounded-full',
              state.mode === 'manual' ? 'bg-warning' : 'bg-muted-foreground/40',
            )}
          />
          {state.mode === 'manual' ? 'Manual' : 'Auto-commit'}
          <Icon name="arrow-down-01" size={10} className="ml-0.5 shrink-0 text-muted-foreground" />
        </DropdownMenuTrigger>
        {state.mode === 'manual' && manualTransactionWarning && (
          <Tip label={manualTransactionWarning}>
            <span className="shrink-0" data-testid="manual-transaction-warning">
              <Icon name="alert-triangle" size={13} className="text-warning" />
            </span>
          </Tip>
        )}
        <DropdownMenuContent align="start">
          <DropdownMenuItem onClick={() => void selectMode('auto')}>
            <Icon name="refresh" size={13} data-icon="inline-start" />
            Auto-commit
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => void selectMode('manual')}>
            <Icon name="pencil-edit-02" size={13} data-icon="inline-start" />
            Manual transaction
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {state.mode === 'manual' && state.open && (
        <>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={state.statements.length === 0}
            className="h-7 px-2 text-xs font-normal text-muted-foreground"
            onClick={openDetails}
          >
            {state.pendingStatements} pending
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-7 px-2 text-xs font-normal"
            onClick={() => void commit()}
          >
            <Icon name="checkmark-circle-02" size={12} data-icon="inline-start" />
            Commit
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-7 px-2 text-xs font-normal text-destructive hover:text-destructive"
            onClick={() => void rollback()}
          >
            <Icon name="arrow-turn-backward" size={12} data-icon="inline-start" />
            Rollback
          </Button>
        </>
      )}

      <Dialog open={detailsOpen} onOpenChange={setDetailsOpen}>
        <DialogContent className="flex flex-col sm:max-w-3xl">
          <DialogHeader>
            <DialogTitle>
              Pending statement{state.statements.length === 1 ? '' : 's'} ({state.statements.length}
              )
            </DialogTitle>
          </DialogHeader>
          <div className="flex max-h-[60vh] min-h-40 overflow-hidden rounded-md border border-border">
            <div
              role="listbox"
              aria-label="Pending statements"
              className="flex w-52 shrink-0 flex-col overflow-y-auto border-r border-border bg-sidebar"
            >
              {state.statements.map((statement, index) => {
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
              <ReadOnlySqlView value={state.statements[selectedStatement] ?? ''} />
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
