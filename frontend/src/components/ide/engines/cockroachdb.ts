import cockroachdbIcon from '#/assets/drivers/cockroachdb.png'
import { cockroachdbDriver } from '../connection-drivers/cockroachdb'
import { postgresHooks } from '../object-detail/drivers/postgres'
import { postgresDialect } from './postgres/dialect'
import { standardTlsSpec } from './tls'
import type { FrontendEngine } from './types'

export const cockroachdbEngine: FrontendEngine = {
  id: 'cockroachdb',
  label: 'CockroachDB',
  brand: { icon: cockroachdbIcon, description: 'Distributed SQL database' },
  dialect: postgresDialect,
  objectDetail: postgresHooks,
  diagram: {},
  connection: cockroachdbDriver,
  tls: standardTlsSpec,
  sshTunnel: true,
  semanticCompletion: true,
}
