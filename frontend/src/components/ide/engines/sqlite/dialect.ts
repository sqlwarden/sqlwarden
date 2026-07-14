import { BaseSqlDialect, createIdentifierQuoter } from '../../dialect'

class SqliteDialect extends BaseSqlDialect {
  private quoteIdentifier = createIdentifierQuoter('"')

  formatObject(_namespace: string, name: string): string {
    return this.quoteIdentifier(name)
  }

  formatColumn(name: string): string {
    return this.quoteIdentifier(name)
  }
}

export const sqliteDialect = new SqliteDialect()
