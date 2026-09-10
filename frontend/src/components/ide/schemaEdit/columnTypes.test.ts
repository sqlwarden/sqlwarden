import { describe, expect, it } from 'vitest'
import { editor } from './schemaEdit.fixtures'
import { canonicalColumnType, validCustomColumnTypeSyntax } from './columnTypes'

describe('canonicalColumnType', () => {
  it.each([
    ['number(10, 2)', 'NUMBER(10,2)'],
    ['VARCHAR2(120)', 'VARCHAR2(120)'],
    ['number(38,-84)', 'NUMBER(38,-84)'],
  ])('canonicalizes %s', (input, expected) => {
    expect(canonicalColumnType(input, editor.column_types, editor.parameterized_column_types)).toBe(
      expected,
    )
  })
  it.each([
    'NUMBER(39)',
    'NUMBER(10,128)',
    'NUMBER(10); DROP TABLE x',
    'NUMBER(10) NOT NULL',
    'NUMBER(10,2,3)',
    'NUMBER()',
    'NUMBER(1.5)',
    'VARCHAR2(4001)',
  ])('rejects %s', (input) => {
    expect(
      canonicalColumnType(input, editor.column_types, editor.parameterized_column_types),
    ).toBeNull()
  })
  it('keeps a closed palette for drivers without parameterized types', () => {
    expect(canonicalColumnType('text', ['TEXT'])).toBe('TEXT')
    expect(canonicalColumnType('NUMBER(10)', ['NUMBER'])).toBeNull()
  })
  it('rejects an unknown type when custom types are not allowed', () => {
    expect(canonicalColumnType('vector(1536)', ['text'], [], false)).toBeNull()
  })
  it('accepts an unknown extension type when custom types are allowed', () => {
    expect(canonicalColumnType('vector(1536)', ['text'], [], true)).toBe('vector(1536)')
    expect(canonicalColumnType('public.hstore', ['text'], [], true)).toBe('public.hstore')
  })
  it('rejects a malformed known type even when custom types are allowed', () => {
    expect(
      canonicalColumnType(
        'NUMBER(39)',
        editor.column_types,
        editor.parameterized_column_types,
        true,
      ),
    ).toBeNull()
  })
  it('rejects unsafe custom type text even when custom types are allowed', () => {
    expect(canonicalColumnType('text); drop table x; --', ['text'], [], true)).toBeNull()
  })
})

describe('validCustomColumnTypeSyntax', () => {
  it.each(['vector', 'vector(1536)', 'public.hstore', 'geometry(Point, 4326)', 'int4[]'])(
    'accepts %s',
    (value) => {
      expect(validCustomColumnTypeSyntax(value)).toBe(true)
    },
  )
  it.each(['', 'text); drop table x; --', "text' OR '1'='1", 'a;b'])('rejects %s', (value) => {
    expect(validCustomColumnTypeSyntax(value)).toBe(false)
  })
})
