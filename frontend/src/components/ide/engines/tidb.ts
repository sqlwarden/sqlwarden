import tidbIcon from '#/assets/drivers/tidb.svg'
import { tidbDriver } from '../connection-drivers/tidb'
import { mysqlHooks } from '../object-detail/drivers/mysql'
import { tidbDialect } from './tidb/dialect'
import { standardTlsSpec } from './tls'
import type { FrontendEngine } from './types'

export const tidbEngine: FrontendEngine = {
  id: 'tidb',
  label: 'TiDB',
  brand: { icon: tidbIcon, description: 'Distributed SQL database' },
  dialect: tidbDialect,
  objectDetail: mysqlHooks,
  diagram: {},
  connection: tidbDriver,
  tls: standardTlsSpec,
  sshTunnel: true,
  manualTransactionWarning:
    'TiDB implicitly commits the transaction on DDL statements (CREATE, ALTER, DROP, TRUNCATE) — those changes cannot be rolled back, and any pending DML commits with them.',
  semanticCompletion: true,
}
