import { sqliteHooks } from '../object-detail/drivers/sqlite'
import { sqliteDialect } from './sqlite/dialect'
import type { FrontendEngine } from './types'

export const sqliteEngine: FrontendEngine = {
  id: 'sqlite',
  label: 'SQLite',
  brand: { description: 'Embedded relational database' },
  dialect: sqliteDialect,
  objectDetail: sqliteHooks,
  diagram: {},
}
