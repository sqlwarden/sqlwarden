import type { AppIcon } from '#/lib/icons'

export interface NavigatorIconStyle {
  icon: AppIcon
  className: string
}

const MUTED = 'text-muted-foreground'
const TABLE = 'text-chart-1/90'
const VIEW = 'text-chart-3/90'
const REMOTE = 'text-chart-4/90'
const CODE = 'text-chart-5/80'

/** Grammar icon token to glyph and accent. Keys must equal metadata.KnownIcons (icons_sync_test.go). */
export const NAVIGATOR_ICONS: Record<string, NavigatorIconStyle> = {
  connection: { icon: 'server-stack-01', className: MUTED },
  database: { icon: 'database', className: MUTED },
  schema: { icon: 'schema', className: MUTED },
  table: { icon: 'table', className: TABLE },
  view: { icon: 'eye', className: VIEW },
  materialized_view: { icon: 'eye', className: VIEW },
  foreign_table: { icon: 'table', className: REMOTE },
  column: { icon: 'column', className: MUTED },
  constraint: { icon: 'checkmark-badge', className: MUTED },
  foreign_key: { icon: 'key-01', className: MUTED },
  index: { icon: 'list-view', className: MUTED },
  dependency: { icon: 'flow-connection', className: MUTED },
  reference: { icon: 'arrow-up-right-01', className: MUTED },
  partition: { icon: 'crop', className: TABLE },
  trigger: { icon: 'flow-connection', className: CODE },
  rule: { icon: 'subject', className: CODE },
  policy: { icon: 'policy', className: MUTED },
  function: { icon: 'play', className: CODE },
  procedure: { icon: 'terminal', className: CODE },
  sequence: { icon: 'sort', className: MUTED },
  type: { icon: 'subject', className: MUTED },
  domain: { icon: 'target', className: MUTED },
  aggregate: { icon: 'pie-chart', className: CODE },
  event_trigger: { icon: 'notification', className: CODE },
  extension: { icon: 'box', className: MUTED },
  event: { icon: 'history', className: CODE },
  user: { icon: 'user-02', className: MUTED },
  role: { icon: 'user-group', className: MUTED },
  profile: { icon: 'user-lock-02', className: MUTED },
  package: { icon: 'briefcase-01', className: CODE },
  queue: { icon: 'arrow-up-down', className: MUTED },
  synonym: { icon: 'copy-01', className: REMOTE },
  db_link: { icon: 'server-stack-01', className: REMOTE },
  extended_property: { icon: 'information-circle', className: MUTED },
}

const FALLBACK: NavigatorIconStyle = { icon: 'box', className: MUTED }

/** Only objects listed directly under a scope are accented; the same token nested under an object (a table's triggers) stays neutral so siblings read consistently. */
export function navigatorIcon(token: string, accented: boolean): NavigatorIconStyle {
  const style = NAVIGATOR_ICONS[token] ?? FALLBACK
  return accented ? style : { icon: style.icon, className: MUTED }
}
