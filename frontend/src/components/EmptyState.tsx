import { Icon, type AppIcon } from '#/lib/icons'
import { TableCell, TableRow } from '#/components/ui/table'

type EmptyStateProps = {
  icon?: AppIcon
  message: string
  description?: string
}

export function EmptyState({ description, icon, message }: EmptyStateProps) {
  return (
    <div className="flex min-h-56 flex-col items-center justify-center gap-3 text-center">
      {icon ? <Icon name={icon} size={40} className="text-muted-foreground" /> : null}
      <div className="flex flex-col gap-1">
        <p className="font-medium text-foreground">{message}</p>
        {description ? <p className="text-sm text-muted-foreground">{description}</p> : null}
      </div>
    </div>
  )
}

type TableEmptyStateProps = EmptyStateProps & {
  colSpan: number
  compact?: boolean
}

export function TableEmptyState({ colSpan, compact = false, icon, message }: TableEmptyStateProps) {
  return (
    <TableRow>
      <TableCell colSpan={colSpan} className={compact ? 'py-10 text-center text-sm text-muted-foreground' : undefined}>
        {compact ? message : <EmptyState icon={icon} message={message} />}
      </TableCell>
    </TableRow>
  )
}
