import { BaseSqlDialect, createIdentifierQuoter } from '../../dialect'
import type { ScopePath } from '#/lib/api/types'
import { scopeName } from '#/lib/api/scope'

class PostgresDialect extends BaseSqlDialect {
  private quoteIdentifier = createIdentifierQuoter('"')

  formatObject(scope: ScopePath, name: string): string {
    const schemaName = scopeName(scope, 'schema')
    const object = this.quoteIdentifier(name)
    return schemaName && schemaName !== 'public'
      ? `${this.quoteIdentifier(schemaName)}.${object}`
      : object
  }

  formatColumn(name: string): string {
    return this.quoteIdentifier(name)
  }
}

export const postgresDialect = new PostgresDialect()
