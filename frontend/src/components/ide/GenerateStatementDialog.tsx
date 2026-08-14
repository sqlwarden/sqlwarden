import { useQuery } from '@tanstack/react-query'
import { Badge } from '#/components/ui/badge'
import { Button } from '#/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '#/components/ui/dialog'
import { Icon } from '#/lib/icons'
import { isApiError } from '#/lib/api/errors'
import { orgConnectionGenerateStatementQueryOptions } from '#/lib/api/query'
import { scopeLabel } from '#/lib/api/scope'
import type { ObjectRef, StatementOperation } from '#/lib/api/types'
import { copyWithToast } from './contextMenus/clipboard'
import { ReadOnlySqlView } from './object-detail/ReadOnlySqlView'

const OPERATION_LABEL: Record<StatementOperation, string> = {
  select: 'SELECT',
  insert: 'INSERT',
  update: 'UPDATE',
  delete: 'DELETE',
}

export type GenerateStatementTarget = { ref: ObjectRef; operation: StatementOperation }

export type GenerateStatementDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgSlug: string
  workspaceId: number
  connectionId: number
  sessionId?: string
  target: GenerateStatementTarget | null
}

/** Read-only preview of a backend-generated SQL statement template for one
 *  schema object. Never inserts or executes the SQL automatically — the user
 *  copies it out explicitly. */
export function GenerateStatementDialog({
  open,
  onOpenChange,
  orgSlug,
  workspaceId,
  connectionId,
  sessionId,
  target,
}: GenerateStatementDialogProps) {
  const query = useQuery({
    ...orgConnectionGenerateStatementQueryOptions(
      orgSlug,
      workspaceId,
      connectionId,
      sessionId,
      target?.operation ?? 'select',
      target?.ref ?? { scope: [], kind: '', name: '' },
    ),
    enabled: open && target !== null,
  })

  const sql = query.data?.sql

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[70vh] flex-col sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Generate SQL</DialogTitle>
          {target && (
            <DialogDescription className="flex items-center gap-1.5">
              <Badge variant="outline" className="font-mono">
                {OPERATION_LABEL[target.operation]}
              </Badge>
              <span className="truncate">
                {[scopeLabel(target.ref.scope), target.ref.name].filter(Boolean).join(' / ')}
              </span>
            </DialogDescription>
          )}
        </DialogHeader>

        <div className="min-h-40 flex-1 overflow-hidden rounded-md border border-border bg-muted/30">
          {query.isLoading ? (
            <div className="flex h-40 items-center justify-center gap-2 text-xs text-muted-foreground">
              <Icon name="loading-03" size={14} className="animate-spin" />
              Generating SQL…
            </div>
          ) : query.isError ? (
            <div className="flex h-40 flex-col items-center justify-center gap-2 px-4 text-center text-xs text-muted-foreground">
              <span>
                {isApiError(query.error) ? query.error.message : 'Failed to generate SQL.'}
              </span>
              <button
                type="button"
                className="text-primary underline hover:no-underline"
                onClick={() => query.refetch()}
              >
                Retry
              </button>
            </div>
          ) : (
            <ReadOnlySqlView value={sql ?? ''} className="h-64" />
          )}
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            size="sm"
            disabled={!sql}
            onClick={() => sql && copyWithToast(sql, 'SQL copied')}
          >
            <Icon name="copy-01" size={13} />
            Copy
          </Button>
          <Button size="sm" onClick={() => onOpenChange(false)}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
