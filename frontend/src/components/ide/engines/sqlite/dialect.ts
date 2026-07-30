import { BaseSqlDialect, createIdentifierQuoter } from '../../dialect'
import type { ScopePath } from '#/lib/api/types'

class SqliteDialect extends BaseSqlDialect {
  private quoteIdentifier = createIdentifierQuoter('"')

  formatObject(_scope: ScopePath, name: string): string {
    return this.quoteIdentifier(name)
  }

  formatColumn(name: string): string {
    return this.quoteIdentifier(name)
  }
}

export const sqliteDialect = new SqliteDialect()
