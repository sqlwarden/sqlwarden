import { networkLayout } from './layouts'
import type { DriverDef } from './types'

export const cockroachdbDriver: DriverDef = {
  id: 'cockroachdb',
  label: 'CockroachDB',
  fields: networkLayout({
    databasePlaceholder: 'defaultdb',
    usernamePlaceholder: 'root',
  }),
}
