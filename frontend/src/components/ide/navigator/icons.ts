import type { AppIcon } from '#/lib/icons'

export interface NavigatorIconStyle {
  icon: AppIcon
  className: string
}

/** Grammar icon token to glyph and accent. Keys must equal metadata.KnownIcons (icons_sync_test.go). */
export const NAVIGATOR_ICONS: Record<string, NavigatorIconStyle> = {
  connection: { icon: 'server-stack-01', className: 'text-muted-foreground' },
  database: { icon: 'database', className: 'text-muted-foreground' },
  schema: { icon: 'folder', className: 'text-muted-foreground' },
  table: { icon: 'table', className: 'text-chart-4' },
  view: { icon: 'eye', className: 'text-chart-2' },
  materialized_view: { icon: 'eye', className: 'text-chart-2' },
  foreign_table: { icon: 'table', className: 'text-chart-2' },
  column: { icon: 'column', className: 'text-muted-foreground' },
  constraint: { icon: 'checkmark-badge', className: 'text-chart-3' },
  foreign_key: { icon: 'key-01', className: 'text-chart-2' },
  index: { icon: 'list-view', className: 'text-chart-3' },
  dependency: { icon: 'flow-connection', className: 'text-muted-foreground' },
  reference: { icon: 'arrow-up-right-01', className: 'text-muted-foreground' },
  partition: { icon: 'crop', className: 'text-chart-4' },
  trigger: { icon: 'flow-connection', className: 'text-chart-5' },
  rule: { icon: 'subject', className: 'text-chart-5' },
  policy: { icon: 'policy', className: 'text-chart-5' },
  function: { icon: 'play', className: 'text-chart-1' },
  procedure: { icon: 'terminal', className: 'text-chart-1' },
  sequence: { icon: 'sort', className: 'text-chart-3' },
  type: { icon: 'subject', className: 'text-chart-3' },
  domain: { icon: 'target', className: 'text-chart-3' },
  aggregate: { icon: 'pie-chart', className: 'text-chart-1' },
  event_trigger: { icon: 'notification', className: 'text-chart-5' },
  extension: { icon: 'box', className: 'text-muted-foreground' },
  event: { icon: 'history', className: 'text-chart-5' },
  user: { icon: 'user-02', className: 'text-muted-foreground' },
  role: { icon: 'user-group', className: 'text-muted-foreground' },
  profile: { icon: 'user-lock-02', className: 'text-muted-foreground' },
  package: { icon: 'briefcase-01', className: 'text-chart-1' },
  queue: { icon: 'arrow-up-down', className: 'text-chart-5' },
  synonym: { icon: 'copy-01', className: 'text-muted-foreground' },
  extended_property: { icon: 'information-circle', className: 'text-muted-foreground' },
}

const FALLBACK: NavigatorIconStyle = { icon: 'box', className: 'text-muted-foreground' }

export function navigatorIcon(token: string): NavigatorIconStyle {
  return NAVIGATOR_ICONS[token] ?? FALLBACK
}
