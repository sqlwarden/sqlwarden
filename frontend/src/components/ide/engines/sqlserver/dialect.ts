import { BaseSqlDialect, createIdentifierQuoter } from '../../dialect'
import type { ObjectRef, ScopePath } from '#/lib/api/types'
import { scopeName } from '#/lib/api/scope'
import { sqlServerSqlFormatter } from '../../sqlFormatter'

class SqlServerDialect extends BaseSqlDialect {
  protected override readonly formatter = sqlServerSqlFormatter
  private quoteIdentifier = createIdentifierQuoter('[', ']')

  formatObject(scope: ScopePath, name: string): string {
    const schemaName = scopeName(scope, 'schema')
    const object = this.quoteIdentifier(name)
    return schemaName && schemaName !== 'dbo'
      ? `${this.quoteIdentifier(schemaName)}.${object}`
      : object
  }

  formatColumn(name: string): string {
    return this.quoteIdentifier(name)
  }

  // T-SQL has no LIMIT clause (spec quirk #2); TOP (n) is the equivalent for
  // bounding the inner row count before COUNT(*). The derived table's
  // projected column needs an explicit alias — T-SQL rejects an unnamed
  // column in a derived table ("No column name was specified for column 1").
  boundedCountQuery(ref: ObjectRef, limit: number): string {
    return `SELECT COUNT(*) FROM (SELECT TOP (${limit}) 1 AS n FROM ${this.formatObject(ref.scope, ref.name)}) AS _warden_count`
  }
}

export const sqlServerDialect = new SqlServerDialect()
