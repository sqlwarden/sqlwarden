import type { ColumnExtra, DriverHooks, HeaderBadge, ObjectViewModel } from '../registry'

function attr(obj: Record<string, unknown> | undefined, key: string): string | undefined {
  const v = obj?.[key]
  return typeof v === 'string' ? v : undefined
}

export const postgresHooks: DriverHooks = {
  headerBadges(vm: ObjectViewModel): HeaderBadge[] {
    const comment = attr(vm.detail.attributes, 'comment')
    return comment ? [{ id: 'comment', label: 'Comment', value: comment }] : []
  },
  columnExtras(): ColumnExtra[] {
    return [{ id: 'comment', header: 'Comment', cell: (col) => attr(col.attributes, 'comment') ?? '' }]
  },
}
