import { networkLayout } from './layouts'
import type { DriverDef } from './types'

export const oracleDriver: DriverDef = {
  id: 'oracle',
  label: 'Oracle',
  fields: networkLayout({
    databaseKey: 'serviceName',
    databaseLabel: 'Service name',
    databasePlaceholder: 'ORCLPDB1',
    usernamePlaceholder: 'system',
  }),
}
