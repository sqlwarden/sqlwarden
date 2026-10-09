import { networkLayout } from './layouts'
import type { DriverDef } from './types'

export const postgresDriver: DriverDef = {
  id: 'postgres',
  label: 'PostgreSQL',
  fields: networkLayout({
    databasePlaceholder: 'Optional',
    usernamePlaceholder: 'postgres',
  }),
}
