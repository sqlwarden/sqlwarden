import type { DriverDef } from './types'

export const sqliteDriver: DriverDef = {
  id: 'sqlite',
  label: 'SQLite',
  defaultPort: 0,
  fields: [
    {
      key: 'path',
      label: 'Database file',
      type: 'text',
      placeholder: 'Leave blank for an in-memory database',
      span: 'full',
      nativeFilePicker: 'sqlite',
    },
  ],
  buildDSN: (values) => values.path?.trim() || ':memory:',
  parseDSN: (dsn) => ({ path: dsn === ':memory:' ? '' : dsn }),
}
