import { useState } from 'react'
import { Icon } from '#/lib/icons'
import { PendingStatementsDialog } from './PendingStatementsDialog'
import { useIde } from './useIdeStore'

/** Status-bar item for the active tab's connection. Renders only while a
 *  manual transaction is open on a live session; clicking it opens the
 *  pending statements dialog. */
export function TransactionStatus({ connectionId }: { connectionId: number | undefined }) {
  const sessionId = useIde((s) => (connectionId ? s.sessions[connectionId] : undefined))
  const state = useIde((s) => (connectionId ? s.transactions[connectionId] : undefined))
  const [detailsOpen, setDetailsOpen] = useState(false)

  if (!sessionId || !state || state.mode !== 'manual' || !state.open) return null

  const count = state.pendingStatements
  return (
    <>
      <button
        type="button"
        onClick={() => setDetailsOpen(true)}
        disabled={state.statements.length === 0}
        className="flex h-full items-center gap-1 rounded-sm px-1.5 font-medium text-warning transition-colors hover:bg-warning/10 disabled:pointer-events-none"
      >
        <Icon name="alert-triangle" size={11} />
        <span className="tabular-nums">
          Transaction open · {count} statement{count === 1 ? '' : 's'}
        </span>
      </button>
      <PendingStatementsDialog
        open={detailsOpen}
        onOpenChange={setDetailsOpen}
        statements={state.statements}
      />
    </>
  )
}
