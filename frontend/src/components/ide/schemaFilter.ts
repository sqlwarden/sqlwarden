import type { DbSchema, DbNamespace, DbObjectGroup, DbObject } from '#/lib/api/types'

/**
 * Prunes a schema to objects matching a case-insensitive substring query.
 * Empty/whitespace query returns the original schema unchanged (same reference).
 * An object is kept whole when its NAME matches; otherwise it is kept with only
 * its matching columns; otherwise dropped. Groups with no surviving objects, and
 * namespaces with no surviving groups, are dropped.
 */
export function filterSchema(schema: DbSchema, query: string): DbSchema {
  const q = query.trim().toLowerCase()
  if (!q) return schema

  const namespaces = (schema.namespaces ?? [])
    .map((ns) => filterNamespace(ns, q))
    .filter((ns) => (ns.object_groups ?? []).length > 0)

  return { ...schema, namespaces }
}

function filterNamespace(ns: DbNamespace, q: string): DbNamespace {
  const object_groups = (ns.object_groups ?? [])
    .map((g) => filterGroup(g, q))
    .filter((g) => (g.objects ?? []).length > 0)
  return { ...ns, object_groups }
}

function filterGroup(g: DbObjectGroup, q: string): DbObjectGroup {
  const objects = (g.objects ?? [])
    .map((o) => filterObject(o, q))
    .filter((o): o is DbObject => o !== null)
  return { ...g, objects }
}

function filterObject(o: DbObject, q: string): DbObject | null {
  if (o.name.toLowerCase().includes(q)) return o
  const columns = (o.columns ?? []).filter((c) => c.name.toLowerCase().includes(q))
  return columns.length > 0 ? { ...o, columns } : null
}
