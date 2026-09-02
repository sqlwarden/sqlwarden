import { describe, expect, it } from 'vitest'
import { oracleDialect } from './dialect'

describe('oracle dialect', () => {
  it('always quotes identifiers', () => {
    expect(oracleDialect.formatColumn('EMPLOYEE_ID')).toBe('"EMPLOYEE_ID"')
    expect(oracleDialect.formatColumn('lower')).toBe('"lower"')
    expect(oracleDialect.formatColumn('a"b')).toBe('"a""b"')
  })

  it('qualifies objects with their schema segment', () => {
    expect(oracleDialect.formatObject([{ kind: 'schema', name: 'HR' }], 'EMPLOYEES')).toBe(
      '"HR"."EMPLOYEES"',
    )
  })

  it('leaves an object unqualified when no schema segment is present', () => {
    expect(oracleDialect.formatObject([], 'EMPLOYEES')).toBe('"EMPLOYEES"')
  })

  it('builds preview and count queries', () => {
    const ref = { scope: [{ kind: 'schema', name: 'HR' }], kind: 'table', name: 'EMPLOYEES' }
    expect(oracleDialect.previewQuery(ref)).toBe('SELECT * FROM "HR"."EMPLOYEES"')
    expect(oracleDialect.exactCountQuery(ref)).toBe('SELECT COUNT(*) FROM "HR"."EMPLOYEES"')
  })

  it('bounds the count query with FETCH FIRST and no alias', () => {
    const ref = { scope: [{ kind: 'schema', name: 'HR' }], kind: 'table', name: 'EMPLOYEES' }
    expect(oracleDialect.boundedCountQuery(ref, 1000)).toBe(
      'SELECT COUNT(*) FROM (SELECT 1 FROM "HR"."EMPLOYEES" FETCH FIRST 1000 ROWS ONLY)',
    )
  })
})
