import { Button } from '#/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '#/components/ui/popover'
import { Icon } from '#/lib/icons'
import { copyWithToast } from './contextMenus/clipboard'
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
    <Popover>
      <PopoverTrigger
        render={
          <button
            type="button"
            className="flex min-w-0 items-center gap-1 text-xs text-destructive hover:underline"
          />
        }
      >
        <Icon name="cancel-01" size={13} className="shrink-0" />
        <span className="truncate">{state.message}</span>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-96 gap-2">
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs font-medium text-destructive">Connection failed</span>
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={() => copyWithToast(state.message)}
            aria-label="Copy error message"
          >
            <Icon name="copy-01" size={12} />
          </Button>
        </div>
        <pre className="max-h-64 overflow-auto text-xs break-words whitespace-pre-wrap text-destructive/90 select-text">
          {state.message}
        </pre>
      </PopoverContent>
    </Popover>
  )
}
