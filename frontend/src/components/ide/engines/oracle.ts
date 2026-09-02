import oracleIcon from '#/assets/drivers/oracle.svg'
import { oracleDriver } from '../connection-drivers/oracle'
import { oracleHooks } from '../object-detail/drivers/oracle'
import { oracleDialect } from './oracle/dialect'
import { standardTlsSpec } from './tls'
import type { FrontendEngine } from './types'

export const oracleEngine: FrontendEngine = {
  id: 'oracle',
  label: 'Oracle',
  brand: { icon: oracleIcon, description: 'Enterprise relational database' },
  dialect: oracleDialect,
  objectDetail: oracleHooks,
  diagram: {},
  connection: oracleDriver,
  tls: standardTlsSpec,
  sshTunnel: true,
  manualTransactionWarning:
    'Oracle commits the current transaction automatically when a DDL statement (CREATE, ALTER, DROP, TRUNCATE) runs — those changes cannot be rolled back, and any pending DML commits with them.',
}
