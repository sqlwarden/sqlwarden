import { BaseSqlDialect, createIdentifierQuoter } from '../../dialect'
import type { ScopePath } from '#/lib/api/types'
import { tidbSqlFormatter } from '../../sqlFormatter'

class TiDbDialect extends BaseSqlDialect {
  protected override readonly formatter = tidbSqlFormatter
  private quoteIdentifier = createIdentifierQuoter('`')
  private quoteCompletionName = createIdentifierQuoter('`', '`', /^[A-Za-z_$][A-Za-z0-9_$]*$/)

  formatObject(_scope: ScopePath, name: string): string {
    return this.quoteIdentifier(name)
  }

  formatColumn(name: string): string {
    return this.quoteIdentifier(name)
  }

  formatIdentifier(name: string): string {
    return this.quoteCompletionName(name)
  }
}

export const tidbDialect = new TiDbDialect()
