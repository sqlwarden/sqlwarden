import { networkLayout } from './layouts'
import type { DriverDef } from './types'

export const tidbDriver: DriverDef = {
  id: 'tidb',
  label: 'TiDB',
  fields: networkLayout({
    databasePlaceholder: 'Optional',
    usernamePlaceholder: 'root',
  }),
}
