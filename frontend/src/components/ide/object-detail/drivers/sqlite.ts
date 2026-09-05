import type { ColumnExtra, ObjectDetailHooks } from '../types'

function attr(obj: Record<string, unknown> | undefined, key: string): string | undefined {
  const v = obj?.[key]
  return typeof v === 'string' ? v : undefined
}

// SQLite renders its DDL/view definition from the base renderer's "source"
// descriptor. The inspector also sets per-column `generated`, `collation`, and
// `check` attributes, surfaced here as extra column cells.
export const sqliteHooks: ObjectDetailHooks = {
  columnExtras(): ColumnExtra[] {
    return [
      {
        id: 'generated',
        header: 'Generated',
        cell: (col) => attr(col.attributes, 'generated') ?? '',
      },
      {
        id: 'collation',
        header: 'Collation',
        cell: (col) => attr(col.attributes, 'collation') ?? '',
      },
      { id: 'check', header: 'Check', cell: (col) => attr(col.attributes, 'check') ?? '' },
    ]
  },
}
