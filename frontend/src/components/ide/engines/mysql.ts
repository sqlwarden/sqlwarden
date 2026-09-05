import mysqlIcon from '#/assets/drivers/mysql.svg'
import { mysqlDriver } from '../connection-drivers/mysql'
import { mysqlHooks } from '../object-detail/drivers/mysql'
import { mysqlDialect } from './mysql/dialect'
import { standardTlsSpec } from './tls'
import type { FrontendEngine } from './types'

export const mysqlEngine: FrontendEngine = {
  id: 'mysql',
  label: 'MySQL',
  brand: { icon: mysqlIcon, description: 'MySQL / MariaDB database' },
  dialect: mysqlDialect,
  objectDetail: mysqlHooks,
  diagram: {},
  connection: mysqlDriver,
  tls: standardTlsSpec,
  sshTunnel: true,
  manualTransactionWarning:
    'MySQL implicitly commits the transaction on DDL statements (CREATE, ALTER, DROP, TRUNCATE) — those changes cannot be rolled back, and any pending DML commits with them.',
  semanticCompletion: true,
}
