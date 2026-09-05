import type { DriverDef } from './types'

// SQLite connects to a local database file. The DSN is the file path in a
// `file:` URI; the modernc.org/sqlite pragmas SQLWarden needs are applied
// server-side, so the form only collects the path.
export const sqliteDriver: DriverDef = {
  id: 'sqlite',
  label: 'SQLite',
  defaultPort: 0,
  fields: [
    {
      key: 'path',
      label: 'Database file path',
      type: 'text',
      placeholder: '/path/to/database.db',
      required: true,
      section: 'Database file',
    },
  ],
  buildDSN: (values) => `file:${values.path}`,
  parseDSN: (dsn): Record<string, string> => {
    const withoutScheme = dsn.replace(/^file:/, '')
    const [path] = withoutScheme.split('?')
    return { path }
  },
}
