import { networkLayout } from './layouts'
import type { DriverDef } from './types'

export const neonDriver: DriverDef = {
  id: 'neon',
  label: 'Neon',
  fields: networkLayout({
    hostPlaceholder: 'ep-xxx-pooler.us-east-2.aws.neon.tech',
    databaseLabel: 'Database',
    databasePlaceholder: 'neondb',
    usernamePlaceholder: 'neondb_owner',
  }),
}
