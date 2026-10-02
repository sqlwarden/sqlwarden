import {
  BaseSqlDialect,
  CASE_INSENSITIVE_BARE_IDENTIFIER,
  createIdentifierQuoter,
} from '../../dialect'
import type { ScopePath } from '#/lib/api/types'
import { scopeName } from '#/lib/api/scope'
import { sqliteSqlFormatter } from '../../sqlFormatter'

class SqliteDialect extends BaseSqlDialect {
  protected override readonly formatter = sqliteSqlFormatter
  private quoteIdentifier = createIdentifierQuoter('"')
  private quoteCompletionName = createIdentifierQuoter('"', '"', CASE_INSENSITIVE_BARE_IDENTIFIER)

  formatObject(scope: ScopePath, name: string): string {
    const database = scopeName(scope, 'database')
    const object = this.quoteIdentifier(name)
    return database && database !== 'main' ? `${this.quoteIdentifier(database)}.${object}` : object
  }

  formatColumn(name: string): string {
    return this.quoteIdentifier(name)
  }

  formatIdentifier(name: string): string {
    return this.quoteCompletionName(name)
  }
}

export const sqliteDialect = new SqliteDialect()
