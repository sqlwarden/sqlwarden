import supabaseIcon from '#/assets/drivers/supabase.svg'
import { supabaseDriver } from '../connection-drivers/supabase'
import { postgresHooks } from '../object-detail/drivers/postgres'
import { postgresDialect } from './postgres/dialect'
import { standardTlsSpec } from './tls'
import type { FrontendEngine } from './types'

export const supabaseEngine: FrontendEngine = {
  id: 'supabase',
  label: 'Supabase',
  brand: { icon: supabaseIcon, description: 'Postgres-based backend platform' },
  dialect: postgresDialect,
  objectDetail: postgresHooks,
  diagram: {},
  connection: supabaseDriver,
  tls: standardTlsSpec,
  sshTunnel: true,
  semanticCompletion: true,
}
