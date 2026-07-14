import { BaseSqlDialect, createIdentifierQuoter } from '../../dialect'

class PostgresDialect extends BaseSqlDialect {
  private quoteIdentifier = createIdentifierQuoter('"')

  formatObject(namespace: string, name: string): string {
    const object = this.quoteIdentifier(name)
    return namespace && namespace !== 'public'
      ? `${this.quoteIdentifier(namespace)}.${object}`
      : object
  }

  formatColumn(name: string): string {
    return this.quoteIdentifier(name)
  }
}

export const postgresDialect = new PostgresDialect()
