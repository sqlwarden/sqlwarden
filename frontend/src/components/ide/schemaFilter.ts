import type { DbSchema, DbNamespace, DbTable, DbView } from '#/lib/api/types'

/**
 * Prunes a schema to objects matching a case-insensitive substring query.
 * Empty/whitespace query returns the original schema unchanged (same reference).
 * A table/view is kept whole when its NAME matches; otherwise it is kept with
 * only its matching columns; otherwise dropped. Namespaces with no surviving
 * tables or views are dropped.
 */
export function filterSchema(schema: DbSchema, query: string): DbSchema {
  const q = query.trim().toLowerCase()
  if (!q) return schema

  const namespaces = (schema.namespaces ?? [])
    .map((ns) => filterNamespace(ns, q))
    .filter((ns) => (ns.tables ?? []).length > 0 || (ns.views ?? []).length > 0)

  return { ...schema, namespaces }
}

function filterNamespace(ns: DbNamespace, q: string): DbNamespace {
  const tables = (ns.tables ?? [])
    .map((t) => filterTable(t, q))
    .filter((t): t is DbTable => t !== null)
  const views = (ns.views ?? [])
    .map((v) => filterView(v, q))
    .filter((v): v is DbView => v !== null)
  return { ...ns, tables, views }
}

function filterTable(t: DbTable, q: string): DbTable | null {
  if (t.name.toLowerCase().includes(q)) return t
  const columns = (t.columns ?? []).filter((c) => c.name.toLowerCase().includes(q))
  return columns.length > 0 ? { ...t, columns } : null
}

function filterView(v: DbView, q: string): DbView | null {
  if (v.name.toLowerCase().includes(q)) return v
  const columns = (v.columns ?? []).filter((c) => c.name.toLowerCase().includes(q))
  return columns.length > 0 ? { ...v, columns } : null
}
