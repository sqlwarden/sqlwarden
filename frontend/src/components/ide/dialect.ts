import type { ObjectRef } from '#/lib/api/types'

/** dataTransfer MIME identifying a schema-identifier drag (vs. a tab drag). */
export const IDENTIFIER_DND_MIME = 'application/x-sqlwarden-identifier'

export interface SqlDialect {
  formatObject(namespace: string, name: string): string
  formatColumn(name: string): string
  previewQuery(ref: ObjectRef): string
  exactCountQuery(ref: ObjectRef): string
  boundedCountQuery(ref: ObjectRef, limit: number): string
}

abstract class BaseDialect implements SqlDialect {
  abstract formatObject(namespace: string, name: string): string
  abstract formatColumn(name: string): string

  previewQuery(ref: ObjectRef): string {
    return `SELECT * FROM ${this.formatObject(ref.namespace, ref.name)}`
  }

  exactCountQuery(ref: ObjectRef): string {
    return `SELECT COUNT(*) FROM ${this.formatObject(ref.namespace, ref.name)}`
  }

  boundedCountQuery(ref: ObjectRef, limit: number): string {
    return `SELECT COUNT(*) FROM (SELECT 1 FROM ${this.formatObject(ref.namespace, ref.name)} LIMIT ${limit}) AS _warden_count`
  }
}

const BARE = /^[a-z_][a-z0-9_]*$/

function makeQuoter(quote: string): (name: string) => string {
  return (name) => (BARE.test(name) ? name : quote + name.split(quote).join(quote + quote) + quote)
}

class PostgresDialect extends BaseDialect {
  private q = makeQuoter('"')

  formatObject(namespace: string, name: string): string {
    const object = this.q(name)
    return namespace && namespace !== 'public' ? `${this.q(namespace)}.${object}` : object
  }

  formatColumn(name: string): string {
    return this.q(name)
  }
}

class MySqlDialect extends BaseDialect {
  private q = makeQuoter('`')

  formatObject(_namespace: string, name: string): string {
    return this.q(name)
  }

  formatColumn(name: string): string {
    return this.q(name)
  }
}

class SqliteDialect extends BaseDialect {
  private q = makeQuoter('"')

  formatObject(_namespace: string, name: string): string {
    return this.q(name)
  }

  formatColumn(name: string): string {
    return this.q(name)
  }
}

export const postgresDialect: SqlDialect = new PostgresDialect()
export const mysqlDialect: SqlDialect = new MySqlDialect()
export const sqliteDialect: SqlDialect = new SqliteDialect()
