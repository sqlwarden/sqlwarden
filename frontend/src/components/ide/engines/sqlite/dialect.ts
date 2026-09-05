import { BaseSqlDialect, createIdentifierQuoter } from '../../dialect'
import type { ScopePath } from '#/lib/api/types'
import { scopeName } from '#/lib/api/scope'
import { sqliteSqlFormatter } from '../../sqlFormatter'

class SqliteDialect extends BaseSqlDialect {
  protected override readonly formatter = sqliteSqlFormatter
  private quoteIdentifier = createIdentifierQuoter('"')

  formatObject(scope: ScopePath, name: string): string {
    const database = scopeName(scope, 'database')
    const object = this.quoteIdentifier(name)
    return database && database !== 'main' ? `${this.quoteIdentifier(database)}.${object}` : object
  }

  formatColumn(name: string): string {
    return this.quoteIdentifier(name)
  }
}

export const sqliteDialect = new SqliteDialect()
