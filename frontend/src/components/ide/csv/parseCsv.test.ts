import { describe, expect, it } from 'vitest'
import { CsvParseError, parseCsv } from './parseCsv'

describe('parseCsv', () => {
  it('parses headers and rows without coercing values', () => {
    expect(parseCsv('id,active,amount\n001,false,12.50')).toEqual({
      headers: ['id', 'active', 'amount'],
      rows: [['001', 'false', '12.50']],
      columnCount: 3,
    })
  })

  it('handles BOM, CRLF, quoted commas, escaped quotes, and embedded newlines', () => {
    expect(parseCsv('\ufeffid,notes\r\n1,"hello, ""team"""\r\n2,"two\nlines"\r\n')).toEqual({
      headers: ['id', 'notes'],
      rows: [
        ['1', 'hello, "team"'],
        ['2', 'two\nlines'],
      ],
      columnCount: 2,
    })
  })

  it('preserves empty fields and ragged records while reporting the widest row', () => {
    expect(parseCsv('a,b\n,\n1,2,3')).toEqual({
      headers: ['a', 'b'],
      rows: [
        ['', ''],
        ['1', '2', '3'],
      ],
      columnCount: 3,
    })
  })

  it('distinguishes empty and header-only documents', () => {
    expect(parseCsv('')).toEqual({ headers: [], rows: [], columnCount: 0 })
    expect(parseCsv('id,name\n')).toEqual({
      headers: ['id', 'name'],
      rows: [],
      columnCount: 2,
    })
  })

  it('reports malformed quoted fields with their location', () => {
    expect(() => parseCsv('id,name\n1,"unfinished')).toThrow(CsvParseError)
    expect(() => parseCsv('id,name\n1,"unfinished')).toThrow(
      'Unterminated quoted field at line 2, column 14',
    )
    expect(() => parseCsv('id\n"value"x')).toThrow('Unexpected character after closing quote')
  })
})
