import postgresIcon from '#/assets/drivers/postgresql.svg'
import { postgresDriver } from '../connection-drivers/postgres'
import { postgresHooks } from '../object-detail/drivers/postgres'
import { postgresDialect } from './postgres/dialect'
import { standardTlsSpec } from './tls'
import type { FrontendEngine } from './types'

export const postgresEngine: FrontendEngine = {
  id: 'postgres',
  label: 'PostgreSQL',
  brand: { icon: postgresIcon, description: 'Open-source relational database' },
  dialect: postgresDialect,
  objectDetail: postgresHooks,
  diagram: {},
  connection: postgresDriver,
  tls: standardTlsSpec,
  sshTunnel: true,
}
