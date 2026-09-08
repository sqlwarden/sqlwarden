import { describe, expect, it } from 'vitest'
import { postgresDriver } from './postgres'
import { cockroachdbDriver } from './cockroachdb'
import { mariadbDriver } from './mariadb'
import { mysqlDriver } from './mysql'
import { neonDriver } from './neon'
import { sqliteDriver } from './sqlite'
import { sqlserverDriver } from './sqlserver'
import { supabaseDriver } from './supabase'
import { tidbDriver } from './tidb'
import { yugabyteDriver } from './yugabyte'

describe('postgresDriver.parseDSN', () => {
  it('round-trips fields built by buildDSN', () => {
    const fields = {
      host: 'db.internal',
      port: '5433',
      database: 'analytics',
      username: 'reader',
      password: 'p@ss w/ord',
    }
    const dsn = postgresDriver.buildDSN(fields)
    expect(postgresDriver.parseDSN(dsn)).toEqual(fields)
  })

  it('parses a DSN without a password', () => {
    const dsn = postgresDriver.buildDSN({
      host: 'localhost',
      port: '5432',
      database: 'app',
      username: 'postgres',
      password: '',
    })
    expect(postgresDriver.parseDSN(dsn)).toEqual({
      host: 'localhost',
      port: '5432',
      database: 'app',
      username: 'postgres',
      password: '',
    })
  })

  it('does not emit or parse an sslmode query parameter', () => {
    const dsn = postgresDriver.buildDSN({
      host: 'h',
      port: '5432',
      database: 'db',
      username: 'u',
      password: 'p',
    })
    expect(dsn).not.toContain('sslmode')
    expect(
      postgresDriver.parseDSN('postgresql://u:p@h:5432/db?sslmode=verify-full'),
    ).not.toHaveProperty('sslmode')
  })

  it('returns an empty object for an unparseable DSN', () => {
    expect(postgresDriver.parseDSN('not-a-url')).toEqual({})
  })
})

describe('mysqlDriver.parseDSN', () => {
  it('round-trips fields built by buildDSN', () => {
    const fields = {
      host: 'db.internal',
      port: '3307',
      database: 'analytics',
      username: 'reader',
      password: 'secret',
    }
    const dsn = mysqlDriver.buildDSN(fields)
    expect(mysqlDriver.parseDSN(dsn)).toEqual(fields)
  })

  it('parses a DSN without a password', () => {
    const dsn = mysqlDriver.buildDSN({
      host: 'localhost',
      port: '3306',
      database: 'app',
      username: 'root',
      password: '',
    })
    expect(mysqlDriver.parseDSN(dsn)).toEqual({
      host: 'localhost',
      port: '3306',
      database: 'app',
      username: 'root',
      password: '',
    })
  })

  it('returns an empty object for an unparseable DSN', () => {
    expect(mysqlDriver.parseDSN('not-a-dsn')).toEqual({})
  })
})

describe('mariadbDriver.parseDSN', () => {
  it('round-trips fields built by buildDSN', () => {
    const fields = {
      host: 'db.internal',
      port: '3307',
      database: 'analytics',
      username: 'reader',
      password: 'secret',
    }
    const dsn = mariadbDriver.buildDSN(fields)
    expect(mariadbDriver.parseDSN(dsn)).toEqual(fields)
  })

  it('parses a DSN without a password', () => {
    const dsn = mariadbDriver.buildDSN({
      host: 'localhost',
      port: '3306',
      database: 'app',
      username: 'root',
      password: '',
    })
    expect(mariadbDriver.parseDSN(dsn)).toEqual({
      host: 'localhost',
      port: '3306',
      database: 'app',
      username: 'root',
      password: '',
    })
  })

  it('returns an empty object for an unparseable DSN', () => {
    expect(mariadbDriver.parseDSN('not-a-dsn')).toEqual({})
  })
})

describe('neonDriver.parseDSN', () => {
  it('round-trips fields built by buildDSN', () => {
    const fields = {
      host: 'ep-cool-lab-12345.us-east-2.aws.neon.tech',
      port: '5432',
      database: 'neondb',
      username: 'neondb_owner',
      password: 'p@ss w/ord',
    }
    const dsn = neonDriver.buildDSN(fields)
    expect(neonDriver.parseDSN(dsn)).toEqual(fields)
  })

  it('parses a DSN without a password', () => {
    const dsn = neonDriver.buildDSN({
      host: 'ep-cool-lab-12345.us-east-2.aws.neon.tech',
      port: '5432',
      database: 'neondb',
      username: 'neondb_owner',
      password: '',
    })
    expect(neonDriver.parseDSN(dsn)).toEqual({
      host: 'ep-cool-lab-12345.us-east-2.aws.neon.tech',
      port: '5432',
      database: 'neondb',
      username: 'neondb_owner',
      password: '',
    })
  })

  it('returns an empty object for an unparseable DSN', () => {
    expect(neonDriver.parseDSN('not-a-url')).toEqual({})
  })
})

describe('cockroachdbDriver.parseDSN', () => {
  it('round-trips fields built by buildDSN', () => {
    const fields = {
      host: 'localhost',
      port: '26257',
      database: 'defaultdb',
      username: 'root',
      password: 'p@ss w/ord',
    }
    const dsn = cockroachdbDriver.buildDSN(fields)
    expect(cockroachdbDriver.parseDSN(dsn)).toEqual(fields)
  })

  it('parses a DSN without a password', () => {
    const dsn = cockroachdbDriver.buildDSN({
      host: 'localhost',
      port: '26257',
      database: 'defaultdb',
      username: 'root',
      password: '',
    })
    expect(cockroachdbDriver.parseDSN(dsn)).toEqual({
      host: 'localhost',
      port: '26257',
      database: 'defaultdb',
      username: 'root',
      password: '',
    })
  })

  it('returns an empty object for an unparseable DSN', () => {
    expect(cockroachdbDriver.parseDSN('not-a-url')).toEqual({})
  })
})

describe('supabaseDriver.parseDSN', () => {
  it('round-trips fields built by buildDSN', () => {
    const fields = {
      host: 'db.xxxxxxxxxxxx.supabase.co',
      port: '5432',
      database: 'postgres',
      username: 'postgres',
      password: 'p@ss w/ord',
    }
    const dsn = supabaseDriver.buildDSN(fields)
    expect(supabaseDriver.parseDSN(dsn)).toEqual(fields)
  })

  it('parses a DSN without a password', () => {
    const dsn = supabaseDriver.buildDSN({
      host: 'db.xxxxxxxxxxxx.supabase.co',
      port: '5432',
      database: 'postgres',
      username: 'postgres',
      password: '',
    })
    expect(supabaseDriver.parseDSN(dsn)).toEqual({
      host: 'db.xxxxxxxxxxxx.supabase.co',
      port: '5432',
      database: 'postgres',
      username: 'postgres',
      password: '',
    })
  })

  it('returns an empty object for an unparseable DSN', () => {
    expect(supabaseDriver.parseDSN('not-a-url')).toEqual({})
  })
})

describe('tidbDriver.parseDSN', () => {
  it('round-trips fields built by buildDSN', () => {
    const fields = {
      host: 'db.internal',
      port: '4000',
      database: 'analytics',
      username: 'reader',
      password: 'secret',
    }
    const dsn = tidbDriver.buildDSN(fields)
    expect(tidbDriver.parseDSN(dsn)).toEqual(fields)
  })

  it('parses a DSN without a password', () => {
    const dsn = tidbDriver.buildDSN({
      host: 'localhost',
      port: '4000',
      database: 'app',
      username: 'root',
      password: '',
    })
    expect(tidbDriver.parseDSN(dsn)).toEqual({
      host: 'localhost',
      port: '4000',
      database: 'app',
      username: 'root',
      password: '',
    })
  })

  it('returns an empty object for an unparseable DSN', () => {
    expect(tidbDriver.parseDSN('not-a-dsn')).toEqual({})
  })
})

describe('yugabyteDriver.parseDSN', () => {
  it('round-trips fields built by buildDSN', () => {
    const fields = {
      host: 'localhost',
      port: '5433',
      database: 'yugabyte',
      username: 'yugabyte',
      password: 'p@ss w/ord',
    }
    const dsn = yugabyteDriver.buildDSN(fields)
    expect(yugabyteDriver.parseDSN(dsn)).toEqual(fields)
  })

  it('parses a DSN without a password', () => {
    const dsn = yugabyteDriver.buildDSN({
      host: 'localhost',
      port: '5433',
      database: 'yugabyte',
      username: 'yugabyte',
      password: '',
    })
    expect(yugabyteDriver.parseDSN(dsn)).toEqual({
      host: 'localhost',
      port: '5433',
      database: 'yugabyte',
      username: 'yugabyte',
      password: '',
    })
  })

  it('returns an empty object for an unparseable DSN', () => {
    expect(yugabyteDriver.parseDSN('not-a-url')).toEqual({})
  })
})

describe('sqlserverDriver.parseDSN', () => {
  it('round-trips fields built by buildDSN', () => {
    const fields = {
      host: 'db.internal',
      port: '1434',
      database: 'analytics',
      username: 'reader',
      password: 'p@ss w/ord',
    }
    const dsn = sqlserverDriver.buildDSN(fields)
    expect(sqlserverDriver.parseDSN(dsn)).toEqual(fields)
  })

  it('parses a DSN without a password', () => {
    const dsn = sqlserverDriver.buildDSN({
      host: 'localhost',
      port: '1433',
      database: 'master',
      username: 'sa',
      password: '',
    })
    expect(sqlserverDriver.parseDSN(dsn)).toEqual({
      host: 'localhost',
      port: '1433',
      database: 'master',
      username: 'sa',
      password: '',
    })
  })

  it('omits the database query parameter when unset', () => {
    const dsn = sqlserverDriver.buildDSN({
      host: 'localhost',
      port: '1433',
      database: '',
      username: 'sa',
      password: '',
    })
    expect(dsn).not.toContain('database=')
    expect(sqlserverDriver.parseDSN(dsn)).toEqual({
      host: 'localhost',
      port: '1433',
      database: '',
      username: 'sa',
      password: '',
    })
  })

  it('returns an empty object for an unparseable DSN', () => {
    expect(sqlserverDriver.parseDSN('not-a-dsn')).toEqual({})
  })
})

describe('sqliteDriver', () => {
  it('builds and parses a file-path DSN', () => {
    const dsn = sqliteDriver.buildDSN({ path: '/data/app.db' })
    expect(dsn).toContain('/data/app.db')
    expect(sqliteDriver.parseDSN(dsn)).toMatchObject({ path: '/data/app.db' })
  })

  it('round-trips fields built by buildDSN', () => {
    const fields = { path: '/var/lib/sqlwarden/example.db' }
    const dsn = sqliteDriver.buildDSN(fields)
    expect(sqliteDriver.parseDSN(dsn)).toEqual(fields)
  })

  it('drops query pragmas when parsing', () => {
    expect(
      sqliteDriver.parseDSN('file:/data/app.db?cache=shared&_pragma=busy_timeout(5000)'),
    ).toEqual({ path: '/data/app.db' })
  })
})
