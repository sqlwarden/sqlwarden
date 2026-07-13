import { Skeleton } from '#/components/ui/skeleton'
import { TableCell, TableRow } from '#/components/ui/table'

export function RolesTableSkeleton({ showActions }: { showActions: boolean }) {
  return Array.from({ length: 5 }).map((_, index) => (
    <TableRow key={index}>
      <TableCell>
        <div className="flex items-center gap-3">
          <Skeleton className="size-8 rounded-md" />
          <div className="flex flex-col gap-2">
            <Skeleton className="h-4 w-32" />
            <Skeleton className="h-3 w-48" />
          </div>
        </div>
      </TableCell>
      <TableCell><Skeleton className="h-5 w-20 rounded-full" /></TableCell>
      <TableCell><Skeleton className="h-5 w-16 rounded-full" /></TableCell>
      <TableCell><Skeleton className="h-4 w-24" /></TableCell>
      {showActions ? <TableCell className="text-end"><Skeleton className="ms-auto h-8 w-16" /></TableCell> : null}
    </TableRow>
  ))
}
