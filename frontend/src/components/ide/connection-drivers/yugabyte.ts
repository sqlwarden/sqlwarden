import { networkLayout } from './layouts'
import type { DriverDef } from './types'

export const yugabyteDriver: DriverDef = {
  id: 'yugabyte',
  label: 'YugabyteDB',
  fields: networkLayout({
    databasePlaceholder: 'yugabyte',
    usernamePlaceholder: 'yugabyte',
  }),
}
