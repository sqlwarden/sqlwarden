import { describe, expect, it } from 'vitest'
import { editor } from './schemaEdit.fixtures'
import { canonicalColumnType } from './columnTypes'

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
})
