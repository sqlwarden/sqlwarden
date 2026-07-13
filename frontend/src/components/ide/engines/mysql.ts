import mysqlIcon from '#/assets/drivers/mysql.svg'
import { mysqlDriver } from '../connection-drivers/mysql'
import { mysqlDialect } from '../dialect'
import { mysqlHooks } from '../object-detail/drivers/mysql'
import type { FrontendEngine } from './types'

export const mysqlEngine: FrontendEngine = {
  id: 'mysql',
  label: 'MySQL',
  brand: { icon: mysqlIcon, description: 'MySQL / MariaDB database' },
  dialect: mysqlDialect,
  objectDetail: mysqlHooks,
  diagram: {},
  connection: mysqlDriver,
}
