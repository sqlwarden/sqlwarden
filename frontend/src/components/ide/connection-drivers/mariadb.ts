import { networkLayout } from './layouts'
import type { DriverDef } from './types'

export const mariadbDriver: DriverDef = {
  id: 'mariadb',
  label: 'MariaDB',
  fields: networkLayout({
    databasePlaceholder: 'Optional',
    usernamePlaceholder: 'root',
  }),
}
