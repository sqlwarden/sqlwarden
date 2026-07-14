import { describe, expect, it } from 'vitest'
import type { ResultValue } from '#/lib/api/types'
import { cellInRange, formatResultValue, isRowInRange, type CellSelection } from './resultValues'

describe('result selection', () => {
  const selection: CellSelection = {
    anchor: { rowIdx: 3, colIdx: 4 },
    active: { rowIdx: 1, colIdx: 2 },
  }

  it('normalizes reverse cell selections', () => {
    expect(cellInRange(1, 2, selection)).toBe(true)
    expect(cellInRange(2, 3, selection)).toBe(true)
    expect(cellInRange(4, 3, selection)).toBe(false)
    expect(cellInRange(2, 1, selection)).toBe(false)
    expect(cellInRange(2, 3, null)).toBe(false)
  })

  it('checks row membership independently of columns', () => {
    expect(isRowInRange(2, selection)).toBe(true)
    expect(isRowInRange(0, selection)).toBe(false)
    expect(isRowInRange(2, null)).toBe(false)
  })
})

describe('formatResultValue', () => {
  const cases: Array<[ResultValue, { display: string; isNull: boolean; isNumeric: boolean }]> = [
    [{ type: 'null' }, { display: 'NULL', isNull: true, isNumeric: false }],
    [
      { type: 'text', text: 'hello' },
      { display: 'hello', isNull: false, isNumeric: false },
    ],
    [
      { type: 'integer', integer: 42 },
      { display: '42', isNull: false, isNumeric: true },
    ],
    [
      { type: 'float', float: 1.5 },
      { display: '1.5', isNull: false, isNumeric: true },
    ],
    [
      { type: 'decimal', decimal: '9.99' },
      { display: '9.99', isNull: false, isNumeric: true },
    ],
    [
      { type: 'bool', bool: false },
      { display: 'false', isNull: false, isNumeric: false },
    ],
    [
      { type: 'time', time: '2026-01-01' },
      { display: '2026-01-01', isNull: false, isNumeric: false },
    ],
    [
      { type: 'bytes', bytes: [0] },
      { display: '(binary)', isNull: false, isNumeric: false },
    ],
  ]

  it.each(cases)('formats %o', (value, expected) => {
    expect(formatResultValue(value)).toEqual(expected)
  })
})
