import type { ColumnExtra, HeaderBadge, ObjectDetailHooks, ObjectViewModel } from '../types'

function attr(obj: Record<string, unknown> | undefined, key: string): string | undefined {
  const v = obj?.[key]
  return typeof v === 'string' ? v : undefined
}

export const oracleHooks: ObjectDetailHooks = {
  headerBadges(vm: ObjectViewModel): HeaderBadge[] {
    const badges: HeaderBadge[] = []
    for (const [id, label] of [
      ['comment', 'Comment'],
      ['tablespace', 'Tablespace'],
      ['partitioned', 'Partitioned'],
    ]) {
      const value = attr(vm.detail.attributes, id)
      if (value) badges.push({ id, label, value })
    }
    return badges
  },
  columnExtras(): ColumnExtra[] {
    return [
      { id: 'comment', header: 'Comment', cell: (col) => attr(col.attributes, 'comment') ?? '' },
      { id: 'identity', header: 'Identity', cell: (col) => attr(col.attributes, 'identity') ?? '' },
    ]
  },
}
