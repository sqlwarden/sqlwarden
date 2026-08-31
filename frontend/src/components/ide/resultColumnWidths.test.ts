import { describe, expect, it } from 'vitest'
import { columnWidthFromName, distributeColumnWidths } from './resultColumnWidths'

describe('columnWidthFromName', () => {
  it('grows with name length, clamped to a minimum and maximum', () => {
    expect(columnWidthFromName('id')).toBeGreaterThanOrEqual(60)
    expect(columnWidthFromName('id')).toBeLessThan(columnWidthFromName('created_at'))
    expect(columnWidthFromName('a'.repeat(200))).toBe(320)
  })
})

describe('distributeColumnWidths', () => {
  it('leaves widths untouched when the container is narrower than the grid', () => {
    const result = distributeColumnWidths([100, 150], [100, 150], 48, 200)
    expect(result.displayWidths).toEqual([100, 150])
    expect(result.columnGrew).toEqual([false, false])
  })

  it('shares leftover width evenly across every auto column for N columns', () => {
    // used = 48 + 100 + 150 = 298; container 398 leaves 100 extra, split two ways
    const result = distributeColumnWidths([100, 150], [100, 150], 48, 398)
    expect(result.displayWidths).toEqual([150, 200])
    expect(result.columnGrew).toEqual([true, true])
  })

  it('grows a single column to fill the rest of the container', () => {
    const result = distributeColumnWidths([100], [100], 48, 300)
    expect(result.displayWidths).toEqual([252])
    expect(result.columnGrew).toEqual([true])
  })

  it('excludes manually resized columns from growth, keeping their exact width', () => {
    // column 0 was dragged to 250 (no longer matches its default of 100), so
    // only column 1 is auto and absorbs all the leftover space.
    // used = 48 + 250 + 150 = 448; container 500 leaves 52 extra for column 1.
    const result = distributeColumnWidths([250, 150], [100, 150], 48, 500)
    expect(result.displayWidths).toEqual([250, 202])
    expect(result.columnGrew).toEqual([false, true])
  })

  it('does not grow anything once every column has been manually resized', () => {
    const result = distributeColumnWidths([250, 300], [100, 150], 48, 700)
    expect(result.displayWidths).toEqual([250, 300])
    expect(result.columnGrew).toEqual([false, false])
  })
})
