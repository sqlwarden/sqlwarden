import type { ObjectDetail, ObjectRef, Relationship } from '#/lib/api/types'

export const DIAGRAM_MAX_TABLES = 60
export const DIAGRAM_MAX_COLUMNS = 1500

export function refKey(ref: ObjectRef): string {
  return `${ref.namespace} ${ref.kind} ${ref.name}`
}

/** Refs on the other end of a FK edge touching `ref`, in either direction,
 *  excluding refs already on the canvas. */
export function hiddenNeighbors(ref: ObjectRef, edges: Relationship[], present: Set<string>): ObjectRef[] {
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
  tableRefs: ObjectRef[], edges: Relationship[], maxTables: number = DIAGRAM_MAX_TABLES,
): { seed: ObjectRef[]; progressive: boolean } {
  if (tableRefs.length <= maxTables) return { seed: tableRefs, progressive: false }
  return { seed: rankByDegree(tableRefs, edges).slice(0, maxTables), progressive: true }
}

/** Estimated node box size for elk layout — grows with column count. While a
 *  node's columns are still loading (detail undefined) we assume a few rows so
 *  layout leaves room and nodes don't overlap once their columns arrive. */
export function estimateNodeSize(detail: ObjectDetail | undefined, collapsed: boolean): { width: number; height: number } {
  const HEADER = 34
  const ROW = 22
  const LOADING_ROWS = 8
  const cols = detail?.relational?.columns?.length ?? LOADING_ROWS
  return { width: 240, height: collapsed || cols === 0 ? HEADER : HEADER + Math.min(cols, 40) * ROW }
}
