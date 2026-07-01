import type { ObjectDetail, Relationship } from '#/lib/api/types'

/** Per-driver extension seam for the ER diagram, mirroring object-detail
 *  DriverHooks. Generic by default; a driver overrides only what it needs. */
export interface DiagramHooks {
  nodeBadges?(detail: ObjectDetail): string[]
  edgeLabel?(edge: Relationship): string | null
}

const HOOKS: Record<string, DiagramHooks> = {
  postgres: {},
  mysql: {},
  sqlite: {},
}

export function getDiagramHooks(driver: string): DiagramHooks {
  return HOOKS[driver] ?? {}
}
