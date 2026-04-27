import { ArrowDown01Icon, ArrowUp01Icon, ArrowUpDownIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import type { SortOrder } from '#/lib/api/types'

type TableColumnHeaderProps = {
  label: string
  sort?: string
  currentSort?: string
  currentOrder?: SortOrder | string
  onSortChange?: (sort: string) => void
}

export function TableColumnHeader({ label, sort, currentSort, currentOrder, onSortChange }: TableColumnHeaderProps) {
  if (!sort || !onSortChange) {
    return <span className="text-sm font-medium text-muted-foreground">{label}</span>
  }

  return (
    <button
      type="button"
      className="inline-flex cursor-pointer items-center gap-2 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
      onClick={() => onSortChange(sort)}
    >
      {label}
      <SortIcon sort={sort} currentSort={currentSort} currentOrder={currentOrder} />
    </button>
  )
}

function SortIcon({ sort, currentSort, currentOrder }: Required<Pick<TableColumnHeaderProps, 'sort'>> & Pick<TableColumnHeaderProps, 'currentSort' | 'currentOrder'>) {
  if (currentSort !== sort) {
    return <HugeiconsIcon icon={ArrowUpDownIcon} strokeWidth={2} className="size-4" />
  }

  return currentOrder === 'asc'
    ? <HugeiconsIcon icon={ArrowUp01Icon} strokeWidth={2} className="size-4" />
    : <HugeiconsIcon icon={ArrowDown01Icon} strokeWidth={2} className="size-4" />
}
