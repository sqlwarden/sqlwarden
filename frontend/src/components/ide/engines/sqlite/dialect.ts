import { BaseSqlDialect, createIdentifierQuoter } from '../../dialect'
import type { ScopePath } from '#/lib/api/types'
import { sqliteSqlFormatter } from '../../sqlFormatter'

class SqliteDialect extends BaseSqlDialect {
  protected override readonly formatter = sqliteSqlFormatter
  private quoteIdentifier = createIdentifierQuoter('"')

  formatObject(_scope: ScopePath, name: string): string {
    return this.quoteIdentifier(name)
  }

  formatColumn(name: string): string {
    return this.quoteIdentifier(name)
  }
}

export const sqliteDialect = new SqliteDialect()
