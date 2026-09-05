import sqliteIcon from '#/assets/drivers/sqlite.svg'
import { sqliteDriver } from '../connection-drivers/sqlite'
import { sqliteHooks } from '../object-detail/drivers/sqlite'
import { sqliteDialect } from './sqlite/dialect'
import type { FrontendEngine } from './types'

export const sqliteEngine: FrontendEngine = {
  id: 'sqlite',
  label: 'SQLite',
  brand: { icon: sqliteIcon, description: 'Embedded relational database' },
  dialect: sqliteDialect,
  objectDetail: sqliteHooks,
  diagram: {},
  connection: sqliteDriver,
  semanticCompletion: true,
}
