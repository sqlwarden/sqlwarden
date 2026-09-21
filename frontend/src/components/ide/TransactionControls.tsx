import { useState } from 'react'
import { Icon } from '#/lib/icons'
import { Button } from '#/components/ui/button'
import type { TransactionMode } from '#/lib/api/types'
import { cn } from '#/lib/utils'
import { getFrontendEngine } from './engines/registry'
import { PendingStatementsDialog } from './PendingStatementsDialog'
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

const SEGMENTS: { mode: TransactionMode; label: string }[] = [
  { mode: 'auto', label: 'Auto' },
  { mode: 'manual', label: 'Manual' },
]

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
  const txOpen = state.mode === 'manual' && state.open

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
    <div className="flex items-center gap-1">
      <div
        role="group"
        aria-label="Transaction mode"
        className="flex h-7 items-center rounded-md border border-input p-0.5"
      >
        {SEGMENTS.map(({ mode, label }) => {
          const active = state.mode === mode
          const warning = mode === 'manual' ? manualTransactionWarning : undefined
          const showBadge = mode === 'manual' && txOpen
          const button = (
            <button
              key={mode}
              type="button"
              aria-pressed={active}
              onClick={() => void selectMode(mode)}
              className={cn(
                'relative flex h-full items-center gap-1 rounded-[calc(var(--radius-md)-2px)] px-2 text-xs transition-colors outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50',
                active
                  ? mode === 'manual'
                    ? 'bg-warning/15 font-medium text-warning'
                    : 'bg-muted font-medium text-foreground'
                  : 'text-muted-foreground hover:text-foreground',
              )}
            >
              {label}
              {(showBadge || (active && warning)) && (
                <span
                  data-testid={
                    showBadge ? 'pending-statements-badge' : 'manual-transaction-warning'
                  }
                  className="ml-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-warning px-1 text-[10px] leading-none font-semibold tabular-nums text-white"
                >
                  {showBadge ? state.pendingStatements : <Icon name="alert-triangle" size={10} />}
                </span>
              )}
            </button>
          )
          const tip = showBadge
            ? `${state.pendingStatements} pending statement${state.pendingStatements === 1 ? '' : 's'} in the open transaction`
            : warning
          return tip ? (
            <Tip key={mode} label={tip}>
              {button}
            </Tip>
          ) : (
            button
          )
        })}
      </div>

      {txOpen && (
        <>
          <Tip label="View pending statements">
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="View pending statements"
              disabled={state.statements.length === 0}
              onClick={() => setDetailsOpen(true)}
            >
              <Icon name="list-view" size={13} />
            </Button>
          </Tip>
          <Tip label="Commit transaction">
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Commit"
              className="bg-success/10 text-success hover:bg-success/20 hover:text-success"
              onClick={() => void commit()}
            >
              <Icon name="checkmark-circle-02" size={14} />
            </Button>
          </Tip>
          <Tip label="Rollback transaction">
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label="Rollback"
              className="bg-destructive/10 text-destructive hover:bg-destructive/20 hover:text-destructive"
              onClick={() => void rollback()}
            >
              <Icon name="arrow-turn-backward" size={14} />
            </Button>
          </Tip>
        </>
      )}

      <PendingStatementsDialog
        open={detailsOpen}
        onOpenChange={setDetailsOpen}
        statements={state.statements}
      />
    </div>
  )
}
