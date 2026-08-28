import { Button } from '#/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '#/components/ui/dialog'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { DriverBadge } from './DriverBadge'
import { formatExactTime } from './relativeTime'
import { Tip } from './schema-diagram/Tip'

type HistoryQueryDialogRow = {
  sqlText: string
  status: 'ok' | 'error' | 'cancelled'
  executedAt: string
}

function statusLabel(status: HistoryQueryDialogRow['status']): string {
  switch (status) {
    case 'ok':
      return 'Succeeded'
    case 'error':
      return 'Failed'
    default:
      return 'Cancelled'
  }
}

function statusTextClass(status: HistoryQueryDialogRow['status']): string {
  switch (status) {
    case 'ok':
      return 'text-green-600 dark:text-green-400'
    case 'error':
      return 'text-destructive'
    default:
      return 'text-muted-foreground'
  }
}

type HistoryQueryDialogProps = {
  row: HistoryQueryDialogRow | null
  connectionName?: string
  driver?: string
  isFavorited: boolean
  canDelete: boolean
  onOpenChange: (open: boolean) => void
  onToggleFavorite: () => void
  onCopy: () => void
  onInsert: () => void
  onDelete: () => void
}

export function HistoryQueryDialog({
  row,
  connectionName,
  driver,
  isFavorited,
  canDelete,
  onOpenChange,
  onToggleFavorite,
  onCopy,
  onInsert,
  onDelete,
}: HistoryQueryDialogProps) {
  return (
    <Dialog open={row !== null} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            {driver && <DriverBadge driver={driver} size="sm" className="size-3.5" />}
            <span className="truncate">{connectionName ?? 'Unknown connection'}</span>
          </DialogTitle>
        </DialogHeader>

        {row && (
          <>
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <span className={cn('font-medium', statusTextClass(row.status))}>
                {statusLabel(row.status)}
              </span>
              <span aria-hidden="true">·</span>
              <span>{formatExactTime(row.executedAt)}</span>
            </div>

            <pre className="max-h-80 overflow-auto rounded-md bg-muted/40 p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap break-words text-foreground">
              {row.sqlText}
            </pre>
          </>
        )}

        <DialogFooter className="justify-between sm:justify-between">
          <div className="flex items-center gap-1">
            <Tip label={isFavorited ? 'Remove from favorites' : 'Save as favorite'}>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label={isFavorited ? 'Remove from favorites' : 'Save as favorite'}
                onClick={onToggleFavorite}
                className={isFavorited ? 'text-amber-500 dark:text-amber-400' : undefined}
              >
                {isFavorited ? (
                  <svg
                    viewBox="0 0 24 24"
                    width={14}
                    height={14}
                    fill="currentColor"
                    aria-hidden="true"
                  >
                    <path d="M10.788 3.21c.448-1.077 1.976-1.077 2.424 0l2.082 5.007 5.404.433c1.164.093 1.636 1.545.749 2.305l-4.117 3.527 1.257 5.273c.271 1.136-.964 2.033-1.96 1.425L12 18.354 7.373 21.18c-.996.608-2.231-.29-1.96-1.425l1.257-5.273-4.117-3.527c-.887-.76-.415-2.212.749-2.305l5.404-.433 2.082-5.007Z" />
                  </svg>
                ) : (
                  <Icon name="star" size={14} />
                )}
              </Button>
            </Tip>
            <Tip label="Copy query">
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="Copy query"
                onClick={onCopy}
              >
                <Icon name="copy-01" size={14} />
              </Button>
            </Tip>
            <Tip label="Insert at cursor">
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="Insert query at cursor"
                onClick={onInsert}
              >
                <Icon name="text-cursor" size={14} />
              </Button>
            </Tip>
            {canDelete && (
              <Tip label="Delete from history">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  aria-label="Delete history entry"
                  onClick={onDelete}
                >
                  <Icon name="delete-01" size={14} />
                </Button>
              </Tip>
            )}
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
