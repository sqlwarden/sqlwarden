import type { ObjectRef } from '#/lib/api/types'

/** dataTransfer MIME identifying a schema-identifier drag (vs. a tab drag). */
export const IDENTIFIER_DND_MIME = 'application/x-sqlwarden-identifier'

/**
 * Per-database rules for turning schema nodes into SQL text. Each driver
 * implements its own quoting, qualification, and query-shape policy; adding a
 * new database means implementing this interface (or extending BaseDialect and
 * overriding only what differs), not editing a global switch.
 */
export interface SqlDialect {
  /** Insert text for a database object (table/view/function/…), qualified with
   *  its namespace when this database requires it. */
  formatObject(namespace: string, name: string): string
  /** Insert text for a column (bare, quoted only if its shape requires it). */
  formatColumn(name: string): string
  /** A SELECT of all rows of an object (paging is handled by the server cursor). */
  previewQuery(ref: ObjectRef): string
  /** An exact `COUNT(*)` of an object. */
  exactCountQuery(ref: ObjectRef): string
  /** A `COUNT(*)` bounded to at most `limit` rows, so it stays cheap on large
   *  objects. Drivers whose pagination is not `LIMIT n` override this. */
  boundedCountQuery(ref: ObjectRef, limit: number): string
}

/**
 * ANSI/`LIMIT`-style query templates shared by the bundled drivers. Subclasses
 * supply identifier quoting (formatObject/formatColumn); they inherit these
 * query shapes and override any that differ for their dialect.
 */
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

// A name safe to use unquoted: starts with a lowercase letter or underscore,
// then lowercase letters, digits, or underscores. Uppercase is excluded so that
// case-folding dialects (Postgres) preserve the original; quoting is harmless
// elsewhere. Quote char is escaped by doubling.
const BARE = /^[a-z_][a-z0-9_]*$/
function makeQuoter(quote: string): (name: string) => string {
  return (name) => (BARE.test(name) ? name : quote + name.split(quote).join(quote + quote) + quote)
}

// Postgres: schemas, default search_path schema is `public`. Unquoted identifiers
// fold to lowercase, so non-lowercase names are quoted. Objects outside `public`
// are schema-qualified so they resolve regardless of search_path.
// (Default schema is treated as `public`; reading search_path is a future refinement.)
class PostgresDialect extends BaseDialect {
  private q = makeQuoter('"')
  formatObject(namespace: string, name: string): string {
    const obj = this.q(name)
    return namespace && namespace !== 'public' ? `${this.q(namespace)}.${obj}` : obj
  }
  formatColumn(name: string): string {
    return this.q(name)
  }
}

// MySQL: schema inspection surfaces only the current database (one namespace), so
// object references never need qualifying. Identifiers are backtick-quoted.
class MySqlDialect extends BaseDialect {
  private q = makeQuoter('`')
  formatObject(_namespace: string, name: string): string {
    return this.q(name)
  }
  formatColumn(name: string): string {
    return this.q(name)
  }
}

// SQLite: single implicit `main` namespace; never qualify. Double-quoted.
class SqliteDialect extends BaseDialect {
  private q = makeQuoter('"')
  formatObject(_namespace: string, name: string): string {
    return this.q(name)
  }
  formatColumn(name: string): string {
    return this.q(name)
  }
}

const postgres = new PostgresDialect()
const DIALECTS: Record<string, SqlDialect> = {
  postgres,
  mysql: new MySqlDialect(),
  sqlite: new SqliteDialect(),
}

/** Returns the dialect for a driver, falling back to the Postgres rules. */
export function dialectFor(driver: string): SqlDialect {
  return DIALECTS[driver] ?? postgres
}
