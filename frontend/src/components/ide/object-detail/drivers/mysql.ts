import type { ColumnExtra, DriverHooks, HeaderBadge, ObjectViewModel } from '../registry'

function attr(obj: Record<string, unknown> | undefined, key: string): string | undefined {
  const v = obj?.[key]
  return typeof v === 'string' ? v : undefined
}

export const mysqlHooks: DriverHooks = {
  headerBadges(vm: ObjectViewModel): HeaderBadge[] {
    const a = vm.detail.attributes
    const out: HeaderBadge[] = []
    const engine = attr(a, 'engine')
    const collation = attr(a, 'collation')
    // Row count is shown as an exact COUNT(*) in the Data section, not the
    // approximate information_schema estimate, so it is intentionally not a badge.
    if (engine) out.push({ id: 'engine', label: 'Engine', value: engine })
    if (collation) out.push({ id: 'collation', label: 'Collation', value: collation })
    return out
  },
  columnExtras(): ColumnExtra[] {
    return [
      { id: 'comment', header: 'Comment', cell: (col) => attr(col.attributes, 'comment') ?? '' },
      { id: 'extra', header: 'Extra', cell: (col) => attr(col.attributes, 'extra') ?? '' },
    ]
  },
}
