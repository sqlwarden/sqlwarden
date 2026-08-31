import type { Connection } from '#/lib/api/types'

export type IdeSearch = { conn?: number }

export function parseIdeSearch(search: Record<string, unknown>): IdeSearch {
  const out: IdeSearch = {}
  const conn = coerceId(search.conn)
  if (conn !== undefined) out.conn = conn
  return out
}

function coerceId(value: unknown): number | undefined {
  const parsed = typeof value === 'string' ? Number(value) : typeof value === 'number' ? value : NaN
  return Number.isInteger(parsed) && parsed > 0 ? parsed : undefined
}

export type DeepLinkResolution = {
  /** Explorer node keys to expand (matches useIdeStore expandedNodes keys). */
  expandKeys: string[]
  /** false = the connection list is still loading; try again when it arrives. */
  ready: boolean
}

/** Pure resolution of ?conn= against loaded connections for the workspace the
 *  route is already scoped to. No auto-connect: opening a live DB session
 *  stays a deliberate user click. */
export function resolveDeepLink(
  search: IdeSearch,
  connections: Connection[] | undefined,
): DeepLinkResolution {
  if (search.conn === undefined) {
    return { expandKeys: [], ready: true }
  }

  if (connections === undefined) {
    return { expandKeys: [], ready: false }
  }

  const conn = connections.find((c) => c.id === search.conn)
  return {
    expandKeys: conn ? [`env:${conn.environment_id}`, `conn:${conn.id}`] : [],
    ready: true,
  }
}
