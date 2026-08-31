/** Icon-tile background/foreground classes for entity avatars (teams, roles,
 *  policy subjects, organizations, etc). A fixed brand-neutral tile — icon
 *  shape, not color, distinguishes entity types (see PolicyTablePrimitives). */
const ENTITY_TILE_CLASS = 'bg-muted text-muted-foreground'

export function entityColor(_name: string): string {
  return ENTITY_TILE_CLASS
}

export const GROUP_COLOR = ENTITY_TILE_CLASS
