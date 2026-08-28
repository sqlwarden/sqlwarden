import { Button } from '#/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '#/components/ui/dialog'
import { Icon } from '#/lib/icons'
import { DriverBadge } from './DriverBadge'
import { formatExactTime } from './relativeTime'
import { Tip } from './schema-diagram/Tip'

type FavoriteQueryDialogRow = {
  name: string
  sqlText: string
  createdAt: string
}

type FavoriteQueryDialogProps = {
  row: FavoriteQueryDialogRow | null
  connectionName?: string
  driver?: string
  onOpenChange: (open: boolean) => void
  onCopy: () => void
  onInsert: () => void
  onDelete: () => void
}

export function FavoriteQueryDialog({
  row,
  connectionName,
  driver,
  onOpenChange,
  onCopy,
  onInsert,
  onDelete,
}: FavoriteQueryDialogProps) {
  return (
    <Dialog open={row !== null} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="truncate">{row?.name}</DialogTitle>
        </DialogHeader>

        {row && (
          <>
            <div className="text-xs text-muted-foreground">
              Saved {formatExactTime(row.createdAt)}
            </div>

            <pre className="max-h-80 overflow-auto rounded-md bg-muted/40 p-3 font-mono text-xs leading-relaxed whitespace-pre-wrap break-words text-foreground">
              {row.sqlText}
            </pre>
          </>
        )}

        <DialogFooter className="justify-between sm:justify-between">
          <div className="flex items-center gap-1">
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
            <Tip label="Delete favorite">
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="Delete favorite"
                onClick={onDelete}
              >
                <Icon name="delete-01" size={14} />
              </Button>
            </Tip>
          </div>

          {connectionName && (
            <span className="flex min-w-0 items-center gap-1 truncate text-xs text-muted-foreground">
              {driver && <DriverBadge driver={driver} size="sm" className="size-3.5" />}
              <span className="truncate">{connectionName}</span>
            </span>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
