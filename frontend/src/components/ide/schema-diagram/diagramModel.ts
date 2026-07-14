import type { ObjectDetail, ObjectRef, Relationship } from '#/lib/api/types'

export const DIAGRAM_MAX_TABLES = 60
export const DIAGRAM_MAX_COLUMNS = 1500

export function refKey(ref: ObjectRef): string {
  return `${ref.namespace} ${ref.kind} ${ref.name}`
}

/** Refs on the other end of a FK edge touching `ref`, in either direction,
 *  excluding refs already on the canvas. */
export function hiddenNeighbors(
  ref: ObjectRef,
  edges: Relationship[],
  present: Set<string>,
): ObjectRef[] {
  const self = refKey(ref)
  const out = new Map<string, ObjectRef>()
  for (const e of edges) {
    let other: ObjectRef | null = null
    if (refKey(e.source) === self) other = e.references
    else if (refKey(e.references) === self) other = e.source
    if (!other) continue
    const k = refKey(other)
    if (k === self || present.has(k)) continue
    out.set(k, other)
  }
  return [...out.values()]
}

/** Sort refs by FK-connection count (degree), descending. */
export function rankByDegree(refs: ObjectRef[], edges: Relationship[]): ObjectRef[] {
  const degree = new Map<string, number>()
  for (const e of edges) {
    degree.set(refKey(e.source), (degree.get(refKey(e.source)) ?? 0) + 1)
    degree.set(refKey(e.references), (degree.get(refKey(e.references)) ?? 0) + 1)
  }
  return [...refs].sort((a, b) => (degree.get(refKey(b)) ?? 0) - (degree.get(refKey(a)) ?? 0))
}

/** Every ref reachable from the seeds by following FK edges in either
 *  direction, out to `maxDepth` hops (default unbounded = the whole connected
 *  component), bounded to `maxTables` so a huge component can't blow past the
 *  render budget. BFS order keeps the seeds and their nearer relations first
 *  when a bound truncates. */
export function reachableRefs(
  seeds: ObjectRef[],
  edges: Relationship[],
  maxTables: number = DIAGRAM_MAX_TABLES,
  maxDepth: number = Infinity,
): ObjectRef[] {
  const adj = new Map<string, ObjectRef[]>()
  const link = (a: ObjectRef, b: ObjectRef) => {
    const ak = refKey(a)
    if (!adj.has(ak)) adj.set(ak, [])
    adj.get(ak)!.push(b)
  }
  for (const e of edges) {
    link(e.source, e.references)
    link(e.references, e.source)
  }
  const result = new Map<string, ObjectRef>()
  const queue: { ref: ObjectRef; depth: number }[] = []
  for (const s of seeds) {
    if (!result.has(refKey(s))) {
      result.set(refKey(s), s)
      queue.push({ ref: s, depth: 0 })
    }
  }
  while (queue.length > 0 && result.size < maxTables) {
    const cur = queue.shift()!
    if (cur.depth >= maxDepth) continue
    for (const nb of adj.get(refKey(cur.ref)) ?? []) {
      if (result.size >= maxTables) break
      const k = refKey(nb)
      if (result.has(k)) continue
      result.set(k, nb)
      queue.push({ ref: nb, depth: cur.depth + 1 })
    }
  }
  return [...result.values()]
}

/** Seed for a namespace diagram: all tables if under the cap, else the most
 *  connected hub tables (progressive mode). */
export function planNamespaceSeed(
  tableRefs: ObjectRef[],
  edges: Relationship[],
  maxTables: number = DIAGRAM_MAX_TABLES,
): { seed: ObjectRef[]; progressive: boolean } {
  if (tableRefs.length <= maxTables) return { seed: tableRefs, progressive: false }
  return { seed: rankByDegree(tableRefs, edges).slice(0, maxTables), progressive: true }
}

/** Estimated node box size for elk layout — grows with column count. While a
 *  node's columns are still loading (detail undefined) we assume a few rows so
 *  layout leaves room and nodes don't overlap once their columns arrive. In
 *  keys-only mode we count just PK/FK columns (plus a row for the "N more"
 *  indicator) so layout matches the compacted node. */
export function estimateNodeSize(
  detail: ObjectDetail | undefined,
  collapsed: boolean,
  keysOnly = false,
): { width: number; height: number } {
  const HEADER = 34
  const ROW = 22
  const LOADING_ROWS = 8
  const rel = detail?.relational
  const allCols = rel?.columns?.length ?? LOADING_ROWS
  let cols = allCols
  if (keysOnly && rel) {
    const keys = new Set<string>(rel.primary_key ?? [])
    for (const f of rel.foreign_keys ?? []) if (f.columns?.[0]) keys.add(f.columns[0])
    cols = keys.size + (keys.size < allCols ? 1 : 0)
  }
  return {
    width: 240,
    height: collapsed || cols === 0 ? HEADER : HEADER + Math.min(cols, 40) * ROW,
  }
}

export type Cardinality = 'one_to_one' | 'one_to_many'

/** Selects a column-specific React Flow handle only after the node internals
 * have been refreshed and the rendered column owns that relationship. */
export function relationshipHandleId({
  column,
  direction,
  handlesReady,
  availableColumns,
  connectedColumns,
}: {
  column?: string
  direction: 'in' | 'out'
  handlesReady: boolean
  availableColumns: ReadonlySet<string>
  connectedColumns: ReadonlySet<string>
}): string {
  if (!handlesReady || !column || !availableColumns.has(column) || !connectedColumns.has(column)) {
    return `node:${direction}`
  }
  return `col:${column}:${direction}`
}

/** Cardinality of a foreign key from the child's side: many children may point
 *  at one parent (one_to_many) unless the child's FK columns are unique — i.e.
 *  they are (or match) the child's primary key or a unique index — in which case
 *  each child maps to exactly one parent (one_to_one). Falls back to one_to_many
 *  when the child's detail (keys/indexes) isn't available. */
export function edgeCardinality(
  fkColumns: string[],
  sourceDetail: ObjectDetail | undefined,
): Cardinality {
  const rel = sourceDetail?.relational
  if (!rel || fkColumns.length === 0) return 'one_to_many'
  const sameSet = (a: string[], b: string[]) =>
    a.length === b.length && [...a].sort().join('\x00') === [...b].sort().join('\x00')
  if (rel.primary_key && sameSet(rel.primary_key, fkColumns)) return 'one_to_one'
  for (const ix of rel.indexes ?? []) {
    if (ix.unique && sameSet(ix.columns ?? [], fkColumns)) return 'one_to_one'
  }
  return 'one_to_many'
}
