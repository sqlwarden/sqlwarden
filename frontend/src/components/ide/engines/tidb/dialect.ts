import { BaseSqlDialect, createIdentifierQuoter } from '../../dialect'
import type { ScopePath } from '#/lib/api/types'
import { tidbSqlFormatter } from '../../sqlFormatter'

class TiDbDialect extends BaseSqlDialect {
  protected override readonly formatter = tidbSqlFormatter
  private quoteIdentifier = createIdentifierQuoter('`')

  formatObject(_scope: ScopePath, name: string): string {
    return this.quoteIdentifier(name)
  }

  formatColumn(name: string): string {
    return this.quoteIdentifier(name)
  }
}

export const tidbDialect = new TiDbDialect()
