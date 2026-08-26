import type { PolicyBinding } from '#/lib/api/types'
import { Icon } from '#/lib/icons'
import { UserAvatar } from '#/components/UserAvatar'
import { Badge } from '#/components/ui/badge'
import { Skeleton } from '#/components/ui/skeleton'
import { TableCell, TableRow } from '#/components/ui/table'
import { entityColor, GROUP_COLOR } from '#/lib/entity-colors'
import { cn } from '#/lib/utils'

type SubjectLabels = Partial<Record<PolicyBinding['subject_type'], string>>

const defaultSubjectLabels: Record<PolicyBinding['subject_type'], string> = {
  account: 'User',
  team: 'Team',
  org_members: 'All users',
  workspace_members: 'All workspace users',
}

export function policySubjectDisplayName(binding: PolicyBinding, labels?: SubjectLabels): string {
  if (binding.subject_type === 'account' || binding.subject_type === 'team') {
    return binding.subject_name || String(binding.subject_id)
  }
  return labels?.[binding.subject_type] ?? defaultSubjectLabels[binding.subject_type]
}

export function PolicySubjectCell({
  binding,
  labels,
}: {
  binding: PolicyBinding
  labels?: SubjectLabels
}) {
  return (
    <div className="flex min-w-0 items-center gap-3">
      <PolicySubjectIcon binding={binding} />
      <div className="min-w-0">
        <div className="truncate font-medium text-foreground">
          {policySubjectDisplayName(binding, labels)}
        </div>
        <div className="mt-0.5 flex items-center gap-1.5">
          <Badge variant="outline" className="h-4 px-1.5 py-0 text-[10px]">
            {labels?.[binding.subject_type] ?? defaultSubjectLabels[binding.subject_type]}
          </Badge>
        </div>
      </div>
    </div>
  )
}

function PolicySubjectIcon({ binding }: { binding: PolicyBinding }) {
  if (binding.subject_type === 'account') {
    return <UserAvatar value={binding.subject_name} fallback="?" />
  }
  if (binding.subject_type === 'team') {
    return (
      <div
        className={cn(
          'flex size-8 shrink-0 items-center justify-center rounded-md',
          entityColor(binding.subject_name),
        )}
      >
        <Icon name="user-group" size={20} className="size-4" />
      </div>
    )
  }
  return (
    <div className={cn('flex size-8 shrink-0 items-center justify-center rounded-md', GROUP_COLOR)}>
      <Icon name="user-multiple-02" size={20} className="size-4" />
    </div>
  )
}

export function PoliciesTableSkeleton({
  canModify,
  showResource = false,
}: {
  canModify: boolean
  showResource?: boolean
}) {
  return Array.from({ length: 5 }).map((_, index) => (
    <TableRow key={index}>
      <TableCell>
        <div className="flex items-center gap-3">
          <Skeleton className="size-8 rounded-md" />
          <div className="flex flex-col gap-2">
            <Skeleton className="h-4 w-36" />
            <Skeleton className="h-3 w-20" />
          </div>
        </div>
      </TableCell>
      {showResource ? (
        <TableCell>
          <div className="flex flex-col gap-2">
            <Skeleton className="h-4 w-32" />
            <Skeleton className="h-3 w-20" />
          </div>
        </TableCell>
      ) : null}
      <TableCell>
        <Skeleton className="h-5 w-24 rounded-md" />
      </TableCell>
      <TableCell>
        <Skeleton className="h-4 w-24" />
      </TableCell>
      {canModify ? (
        <TableCell className="text-end">
          <Skeleton className="ms-auto h-8 w-16" />
        </TableCell>
      ) : null}
    </TableRow>
  ))
}
