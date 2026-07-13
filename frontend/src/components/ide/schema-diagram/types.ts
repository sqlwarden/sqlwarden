import type { ObjectDetail, Relationship } from '#/lib/api/types'

export interface DiagramHooks {
  nodeBadges?(detail: ObjectDetail): string[]
  edgeLabel?(edge: Relationship): string | null
}
