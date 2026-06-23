import { describe, it, expect } from 'vitest'
import { nextCell } from './resultGridNav'

describe('nextCell', () => {
  const at = (rowIdx: number, colIdx: number) => ({ rowIdx, colIdx })

  it('moves within bounds', () => {
    expect(nextCell('ArrowDown', at(2, 1), 10, 5)).toEqual(at(3, 1))
    expect(nextCell('ArrowUp', at(2, 1), 10, 5)).toEqual(at(1, 1))
    expect(nextCell('ArrowRight', at(2, 1), 10, 5)).toEqual(at(2, 2))
    expect(nextCell('ArrowLeft', at(2, 1), 10, 5)).toEqual(at(2, 0))
  })

  it('clamps at the edges', () => {
    expect(nextCell('ArrowUp', at(0, 0), 10, 5)).toEqual(at(0, 0))
    expect(nextCell('ArrowLeft', at(0, 0), 10, 5)).toEqual(at(0, 0))
    expect(nextCell('ArrowDown', at(9, 4), 10, 5)).toEqual(at(9, 4))
    expect(nextCell('ArrowRight', at(9, 4), 10, 5)).toEqual(at(9, 4))
  })

  it('returns null for non-arrow keys', () => {
    expect(nextCell('Enter', at(2, 1), 10, 5)).toBeNull()
    expect(nextCell('a', at(2, 1), 10, 5)).toBeNull()
  })
})
