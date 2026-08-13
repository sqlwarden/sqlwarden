import { Icon } from '#/lib/icons'
import type { ConnectionTestState } from './useConnectionForm'

export function TestStatusIndicator({ state }: { state: ConnectionTestState }) {
  if (state.status === 'idle') return null
  if (state.status === 'pending') {
    return <span className="text-xs text-muted-foreground">Connecting…</span>
  }
  if (state.status === 'ok') {
    return (
      <span className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
        <Icon name="tick-02" size={13} />
        {state.latencyMs}ms
      </span>
    )
  }
  return (
    <span className="flex min-w-0 items-center gap-1 text-xs text-destructive">
      <Icon name="cancel-01" size={13} className="shrink-0" />
      <span className="truncate" title={state.message}>
        {state.message}
      </span>
    </span>
  )
}
