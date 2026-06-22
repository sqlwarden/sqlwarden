import { describe, it, expect } from 'vitest'
import { columnTypeIcon } from './columnTypeIcon'

describe('columnTypeIcon', () => {
  it('maps numeric types', () => {
    for (const t of ['int', 'int4', 'int8', 'bigint', 'smallint', 'serial', 'decimal', 'numeric(10,2)', 'double precision', 'real', 'float8', 'money', 'tinyint']) {
      expect(columnTypeIcon(t)).toBe('type-number')
    }
  })

  it('maps MySQL display-width and unsigned integer forms (PK/FK columns)', () => {
    for (const t of ['bigint(20)', 'int(11)', 'int(11) unsigned', 'bigint unsigned', 'smallint(6)', 'mediumint', 'integer']) {
      expect(columnTypeIcon(t)).toBe('type-number')
    }
  })

  it('does not misclassify "point" (ends in "int") as a number', () => {
    expect(columnTypeIcon('point')).toBe('column')
  })

  it('maps boolean before number (bit/tinyint(1))', () => {
    expect(columnTypeIcon('bool')).toBe('type-boolean')
    expect(columnTypeIcon('boolean')).toBe('type-boolean')
    expect(columnTypeIcon('tinyint(1)')).toBe('type-boolean')
  })

  it('maps string types', () => {
    for (const t of ['varchar', 'varchar(255)', 'character varying', 'char', 'text', 'citext', 'name', 'nvarchar']) {
      expect(columnTypeIcon(t)).toBe('type-string')
    }
  })

  it('distinguishes timestamp, date, and time', () => {
    expect(columnTypeIcon('timestamp')).toBe('type-timestamp')
    expect(columnTypeIcon('timestamptz')).toBe('type-timestamp')
    expect(columnTypeIcon('timestamp without time zone')).toBe('type-timestamp')
    expect(columnTypeIcon('datetime')).toBe('type-timestamp')
    expect(columnTypeIcon('date')).toBe('type-date')
    expect(columnTypeIcon('time')).toBe('type-time')
    expect(columnTypeIcon('timetz')).toBe('type-time')
  })

  it('maps uuid, json, and binary', () => {
    expect(columnTypeIcon('uuid')).toBe('type-uuid')
    expect(columnTypeIcon('json')).toBe('type-json')
    expect(columnTypeIcon('jsonb')).toBe('type-json')
    expect(columnTypeIcon('bytea')).toBe('type-binary')
    expect(columnTypeIcon('blob')).toBe('type-binary')
    expect(columnTypeIcon('varbinary(16)')).toBe('type-binary')
  })

  it('is case-insensitive', () => {
    expect(columnTypeIcon('VARCHAR(20)')).toBe('type-string')
    expect(columnTypeIcon('BIGINT')).toBe('type-number')
  })

  it('falls back to the generic column icon for unknown types', () => {
    expect(columnTypeIcon('geometry')).toBe('column')
    expect(columnTypeIcon('')).toBe('column')
    expect(columnTypeIcon('some_custom_enum')).toBe('column')
  })
})
