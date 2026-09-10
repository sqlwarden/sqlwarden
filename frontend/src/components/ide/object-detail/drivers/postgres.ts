import type { ColumnExtra, HeaderBadge, ObjectDetailHooks, ObjectViewModel } from '../types'

function attr(obj: Record<string, unknown> | undefined, key: string): string | undefined {
  const v = obj?.[key]
  return typeof v === 'string' ? v : undefined
}

export const postgresHooks: ObjectDetailHooks = {
  headerBadges(vm: ObjectViewModel): HeaderBadge[] {
    const badges: HeaderBadge[] = []
    const comment = attr(vm.detail.attributes, 'comment')
    if (comment) badges.push({ id: 'comment', label: 'Comment', value: comment })
    const server = attr(vm.detail.attributes, 'server')
    if (server) badges.push({ id: 'server', label: 'Foreign server', value: server })
    return badges
  },
  columnExtras(): ColumnExtra[] {
    return [
      { id: 'comment', header: 'Comment', cell: (col) => attr(col.attributes, 'comment') ?? '' },
    ]
  },
}
