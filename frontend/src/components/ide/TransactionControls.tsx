import { Button } from '#/components/ui/button'
import { Toggle } from '#/components/ui/toggle'
import { Tip } from './schema-diagram/Tip'
import { useTransactionMode } from './useTransactionMode'

export function TransactionControls({
  orgSlug,
  workspaceId,
  connectionId,
  sessionId,
  onSwitchToAutoBlocked,
}: {
  orgSlug: string
  workspaceId: number
  connectionId: number | undefined
  sessionId: string | undefined
  /** Called instead of switching when a transaction is open — the caller
   *  shows the Commit/Rollback/Cancel guard dialog. */
  onSwitchToAutoBlocked: () => void
}) {
  const { state, switchToManual, switchToAuto, commit, rollback } = useTransactionMode(
    orgSlug,
    workspaceId,
    connectionId,
    sessionId,
  )

  if (!connectionId || !sessionId) return null

  return (
    <div className="flex items-center gap-2">
      <Tip label={state.mode === 'manual' ? 'Manual commit' : 'Auto-commit'}>
        <Toggle
          variant="outline"
          size="sm"
          aria-label="Manual transaction mode"
          pressed={state.mode === 'manual'}
          onPressedChange={async (pressed) => {
            if (pressed) {
              switchToManual()
              return
            }
            const result = await switchToAuto()
            if (result === 'blocked') onSwitchToAutoBlocked()
          }}
        >
          Manual
        </Toggle>
      </Tip>
      {state.mode === 'manual' && state.open && (
        <>
          <span className="rounded-full bg-warning/15 px-1.5 py-0.5 text-[10px] font-medium text-warning">
            {state.pendingStatements} pending
          </span>
          <Button type="button" variant="outline" size="sm" onClick={() => void commit()}>
            Commit
          </Button>
          <Button type="button" variant="outline" size="sm" onClick={() => void rollback()}>
            Rollback
          </Button>
        </>
      )}
    </div>
  )
}
