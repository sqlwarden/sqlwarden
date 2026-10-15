import type { FieldLayout } from './types'

/** Host, port, database, and credentials layout shared by network database engines. */
export function networkLayout(options: {
  hostPlaceholder?: string
  databaseKey?: string
  databaseLabel?: string
  databasePlaceholder?: string
  usernamePlaceholder?: string
}): FieldLayout[] {
  return [
    {
      key: 'host',
      label: 'Host',
      placeholder: options.hostPlaceholder ?? 'localhost',
      section: 'Server',
      span: 'wide',
    },
    { key: 'port', label: 'Port', section: 'Server', span: 'compact' },
    {
      key: options.databaseKey ?? 'database',
      label: options.databaseLabel ?? 'Database (optional)',
      placeholder: options.databasePlaceholder,
      section: 'Server',
    },
    {
      key: 'username',
      label: 'Username',
      placeholder: options.usernamePlaceholder,
      section: 'Credentials',
      span: 'half',
    },
    { key: 'password', label: 'Password', section: 'Credentials', span: 'half' },
  ]
}
