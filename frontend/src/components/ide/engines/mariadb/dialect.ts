import { BaseSqlDialect, createIdentifierQuoter } from '../../dialect'
import type { ScopePath } from '#/lib/api/types'
import { mariadbSqlFormatter } from '../../sqlFormatter'

class MariaDbDialect extends BaseSqlDialect {
  protected override readonly formatter = mariadbSqlFormatter
  private quoteIdentifier = createIdentifierQuoter('`')

  formatObject(_scope: ScopePath, name: string): string {
    return this.quoteIdentifier(name)
  }

  formatColumn(name: string): string {
    return this.quoteIdentifier(name)
  }
}

export const mariadbDialect = new MariaDbDialect()
