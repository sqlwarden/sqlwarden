import type { ObjectRef, ScopePath } from '#/lib/api/types'
import { defaultSqlFormatter, type SqlTextFormatter } from './sqlFormatter'

/** dataTransfer MIME identifying a schema-identifier drag (vs. a tab drag). */
export const IDENTIFIER_DND_MIME = 'application/x-sqlwarden-identifier'

export interface SqlDialect {
  formatSql(sql: string): string
  formatObject(scope: ScopePath, name: string): string
  formatColumn(name: string): string
  previewQuery(ref: ObjectRef): string
  exactCountQuery(ref: ObjectRef): string
  boundedCountQuery(ref: ObjectRef, limit: number): string
}

export abstract class BaseSqlDialect implements SqlDialect {
  protected readonly formatter: SqlTextFormatter = defaultSqlFormatter

  abstract formatObject(scope: ScopePath, name: string): string
  abstract formatColumn(name: string): string

  formatSql(sql: string): string {
    return this.formatter.format(sql)
  }

  previewQuery(ref: ObjectRef): string {
    return `SELECT * FROM ${this.formatObject(ref.scope, ref.name)}`
  }

  exactCountQuery(ref: ObjectRef): string {
    return `SELECT COUNT(*) FROM ${this.formatObject(ref.scope, ref.name)}`
  }

  boundedCountQuery(ref: ObjectRef, limit: number): string {
    return `SELECT COUNT(*) FROM (SELECT 1 FROM ${this.formatObject(ref.scope, ref.name)} LIMIT ${limit}) AS _warden_count`
  }
}

const BARE_IDENTIFIER = /^[a-z_][a-z0-9_]*$/

export function createIdentifierQuoter(
  openingQuote: string,
  closingQuote = openingQuote,
): (name: string) => string {
  return (name) => {
    if (BARE_IDENTIFIER.test(name)) return name
    const escapedName = name.split(closingQuote).join(closingQuote + closingQuote)
    return openingQuote + escapedName + closingQuote
  }
}
