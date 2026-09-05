import neonIcon from '#/assets/drivers/neon.svg'
import { neonDriver } from '../connection-drivers/neon'
import { postgresHooks } from '../object-detail/drivers/postgres'
import { postgresDialect } from './postgres/dialect'
import { standardTlsSpec } from './tls'
import type { FrontendEngine } from './types'

export const neonEngine: FrontendEngine = {
  id: 'neon',
  label: 'Neon',
  brand: { icon: neonIcon, description: 'Serverless Postgres' },
  dialect: postgresDialect,
  objectDetail: postgresHooks,
  diagram: {},
  connection: neonDriver,
  tls: standardTlsSpec,
  sshTunnel: true,
  semanticCompletion: true,
}
