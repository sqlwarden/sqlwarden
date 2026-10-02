import { BaseSqlDialect, createIdentifierQuoter } from '../../dialect'
import type { ScopePath } from '#/lib/api/types'
import { mysqlSqlFormatter } from '../../sqlFormatter'

class MySqlDialect extends BaseSqlDialect {
  protected override readonly formatter = mysqlSqlFormatter
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

export const mysqlDialect = new MySqlDialect()
