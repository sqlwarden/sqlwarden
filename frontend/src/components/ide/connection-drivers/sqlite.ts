import type { DriverDef } from './types'

export const sqliteDriver: DriverDef = {
  id: 'sqlite',
  label: 'SQLite',
  fields: [
    {
      key: 'path',
      label: 'Database file path',
      placeholder: '/path/to/database.db',
      section: 'Database file',
    },
  ],
}
