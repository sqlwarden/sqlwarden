import { BaseSqlDialect } from '../../dialect'
import type { ObjectRef, ScopePath } from '#/lib/api/types'
import { scopeName } from '#/lib/api/scope'
import { oracleSqlFormatter } from '../../sqlFormatter'

class OracleDialect extends BaseSqlDialect {
  protected override readonly formatter = oracleSqlFormatter
  private quoteIdentifier = (name: string) => `"${name.split('"').join('""')}"`

  formatObject(scope: ScopePath, name: string): string {
    const schema = scopeName(scope, 'schema')
    const object = this.quoteIdentifier(name)
    return schema ? `${this.quoteIdentifier(schema)}.${object}` : object
  }

  formatColumn(name: string): string {
    return this.quoteIdentifier(name)
  }

  override boundedCountQuery(ref: ObjectRef, limit: number): string {
    return `SELECT COUNT(*) FROM (SELECT 1 FROM ${this.formatObject(ref.scope, ref.name)} FETCH FIRST ${limit} ROWS ONLY) _warden_count`
  }
}

export const oracleDialect = new OracleDialect()
