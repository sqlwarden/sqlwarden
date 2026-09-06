import yugabyteIcon from '#/assets/drivers/yugabyte.svg'
import { yugabyteDriver } from '../connection-drivers/yugabyte'
import { postgresHooks } from '../object-detail/drivers/postgres'
import { postgresDialect } from './postgres/dialect'
import { standardTlsSpec } from './tls'
import type { FrontendEngine } from './types'

export const yugabyteEngine: FrontendEngine = {
  id: 'yugabyte',
  label: 'YugabyteDB',
  brand: { icon: yugabyteIcon, description: 'Distributed SQL database' },
  dialect: postgresDialect,
  objectDetail: postgresHooks,
  diagram: {},
  connection: yugabyteDriver,
  tls: standardTlsSpec,
  sshTunnel: true,
  semanticCompletion: true,
}
