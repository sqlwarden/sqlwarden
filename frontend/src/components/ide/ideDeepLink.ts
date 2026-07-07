import type { Connection, Workspace } from '#/lib/api/types'

export type IdeSearch = { ws?: number; conn?: number }

export function parseIdeSearch(search: Record<string, unknown>): IdeSearch {
  const out: IdeSearch = {}
  const ws = coerceId(search.ws)
  const conn = coerceId(search.conn)
  if (ws !== undefined) out.ws = ws
  if (conn !== undefined) out.conn = conn
  return out
}

function coerceId(value: unknown): number | undefined {
  const parsed = typeof value === 'string' ? Number(value) : typeof value === 'number' ? value : NaN
  return Number.isInteger(parsed) && parsed > 0 ? parsed : undefined
}

export type DeepLinkResolution = {
  activateWorkspaceId?: number
  /** Explorer node keys to expand (matches useIdeStore expandedNodes keys). */
  expandKeys: string[]
  /** false = the connection list is still loading; try again when it arrives. */
  ready: boolean
}

/** Pure resolution of ?ws=&conn= against loaded data. No auto-connect:
 *  opening a live DB session stays a deliberate user click. */
export function resolveDeepLink(
  search: IdeSearch,
  workspaces: Workspace[],
  connections: Connection[] | undefined,
): DeepLinkResolution {
  const ws = search.ws !== undefined && workspaces.some((w) => w.id === search.ws) ? search.ws : undefined

  if (search.conn === undefined || ws === undefined) {
    return { activateWorkspaceId: ws, expandKeys: [], ready: true }
  }

  if (connections === undefined) {
    return { activateWorkspaceId: ws, expandKeys: [], ready: false }
  }

  const conn = connections.find((c) => c.id === search.conn)
  return {
    activateWorkspaceId: ws,
    expandKeys: conn ? [`env:${conn.environment_id}`, `conn:${conn.id}`] : [],
    ready: true,
  }
}
