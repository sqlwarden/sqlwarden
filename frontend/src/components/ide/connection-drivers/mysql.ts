import { networkLayout } from './layouts'
import type { DriverDef } from './types'

export const mysqlDriver: DriverDef = {
  id: 'mysql',
  label: 'MySQL',
  fields: networkLayout({
    databasePlaceholder: 'Optional',
    usernamePlaceholder: 'root',
  }),
}
