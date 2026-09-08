import type { ColumnExtra, ObjectDetailHooks } from '../types'

function attr(obj: Record<string, unknown> | undefined, key: string): string | undefined {
  const v = obj?.[key]
  return typeof v === 'string' ? v : undefined
}

// catalog.go's setColumnAttr only ever sets "identity" (IDENTITY(seed,increment))
// and "computed" (the computed column's expression) — surfaced here as extra
// column cells. No header-level attributes are emitted yet.
export const sqlServerHooks: ObjectDetailHooks = {
  columnExtras(): ColumnExtra[] {
    return [
      { id: 'identity', header: 'Identity', cell: (col) => attr(col.attributes, 'identity') ?? '' },
      { id: 'computed', header: 'Computed', cell: (col) => attr(col.attributes, 'computed') ?? '' },
    ]
  },
}
