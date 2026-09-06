import mariadbIcon from '#/assets/drivers/mariadb.svg'
import { mariadbDriver } from '../connection-drivers/mariadb'
import { mysqlHooks } from '../object-detail/drivers/mysql'
import { mariadbDialect } from './mariadb/dialect'
import { standardTlsSpec } from './tls'
import type { FrontendEngine } from './types'

export const mariadbEngine: FrontendEngine = {
  id: 'mariadb',
  label: 'MariaDB',
  brand: { icon: mariadbIcon, description: 'MariaDB database' },
  dialect: mariadbDialect,
  objectDetail: mysqlHooks,
  diagram: {},
  connection: mariadbDriver,
  tls: standardTlsSpec,
  sshTunnel: true,
  manualTransactionWarning:
    'MariaDB implicitly commits the transaction on DDL statements (CREATE, ALTER, DROP, TRUNCATE) — those changes cannot be rolled back, and any pending DML commits with them.',
  semanticCompletion: true,
}
