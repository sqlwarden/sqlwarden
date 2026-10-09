import { networkLayout } from './layouts'
import type { DriverDef } from './types'

export const sqlserverDriver: DriverDef = {
  id: 'sqlserver',
  label: 'SQL Server',
  fields: networkLayout({
    databasePlaceholder: 'Optional',
    usernamePlaceholder: 'sa',
  }),
}
