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

/** Seed for an object diagram: the ref plus its 1-hop FK neighbors. */
export function planObjectSeed(ref: ObjectRef, edges: Relationship[]): ObjectRef[] {
  const seed = new Map<string, ObjectRef>([[refKey(ref), ref]])
  for (const n of hiddenNeighbors(ref, edges, new Set([refKey(ref)]))) seed.set(refKey(n), n)
  return [...seed.values()]
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
