import { sqliteDialect } from '../dialect'
import { sqliteHooks } from '../object-detail/drivers/sqlite'
import type { FrontendEngine } from './types'

export const sqliteEngine: FrontendEngine = {
  id: 'sqlite',
  label: 'SQLite',
  brand: { description: 'Embedded relational database' },
  dialect: sqliteDialect,
  objectDetail: sqliteHooks,
  diagram: {},
}
