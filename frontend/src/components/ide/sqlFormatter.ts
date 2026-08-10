import {
  formatDialect,
  mysql,
  postgresql,
  sql as standardSql,
  sqlite,
  type DialectOptions,
} from 'sql-formatter'

export interface SqlTextFormatter {
  format(sql: string): string
}

function formatterFor(dialect: DialectOptions): SqlTextFormatter {
  return {
    format(sql: string) {
      return formatDialect(sql, { dialect })
    },
  }
}

/** Used when an editor has no connection or its engine has no custom formatter. */
export const defaultSqlFormatter = formatterFor(standardSql)

export const mysqlSqlFormatter = formatterFor(mysql)
export const postgresSqlFormatter = formatterFor(postgresql)
export const sqliteSqlFormatter = formatterFor(sqlite)
