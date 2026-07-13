import postgresIcon from '#/assets/drivers/postgresql.svg'
import { postgresDriver } from '../connection-drivers/postgres'
import { postgresDialect } from '../dialect'
import { postgresHooks } from '../object-detail/drivers/postgres'
import type { FrontendEngine } from './types'

export const postgresEngine: FrontendEngine = {
  id: 'postgres',
  label: 'PostgreSQL',
  brand: { icon: postgresIcon, description: 'Open-source relational database' },
  dialect: postgresDialect,
  objectDetail: postgresHooks,
  diagram: {},
  connection: postgresDriver,
}
