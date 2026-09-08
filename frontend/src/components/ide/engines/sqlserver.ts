import sqlserverIcon from '#/assets/drivers/sqlserver.svg'
import { sqlserverDriver } from '../connection-drivers/sqlserver'
import { sqlServerHooks } from '../object-detail/drivers/sqlserver'
import { sqlServerDialect } from './sqlserver/dialect'
import { standardTlsSpec } from './tls'
import type { FrontendEngine } from './types'

export const sqlServerEngine: FrontendEngine = {
  id: 'sqlserver',
  label: 'SQL Server',
  brand: { icon: sqlserverIcon, description: 'Microsoft SQL Server database' },
  dialect: sqlServerDialect,
  objectDetail: sqlServerHooks,
  diagram: {},
  connection: sqlserverDriver,
  tls: standardTlsSpec,
  sshTunnel: true,
  semanticCompletion: true,
}
