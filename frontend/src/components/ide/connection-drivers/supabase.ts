import { networkLayout } from './layouts'
import type { DriverDef } from './types'

export const supabaseDriver: DriverDef = {
  id: 'supabase',
  label: 'Supabase',
  fields: networkLayout({
    hostPlaceholder: 'db.xxxxxxxxxxxx.supabase.co',
    databaseLabel: 'Database',
  }),
}
