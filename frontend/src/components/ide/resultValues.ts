import type { ResultValue } from '#/lib/api/types'

export type CellCoord = { rowIdx: number; colIdx: number }
export type CellSelection = { anchor: CellCoord; active: CellCoord }

export function cellInRange(
  rowIndex: number,
  columnIndex: number,
  selection: CellSelection | null,
): boolean {
  if (!selection) return false
  const minRow = Math.min(selection.anchor.rowIdx, selection.active.rowIdx)
  const maxRow = Math.max(selection.anchor.rowIdx, selection.active.rowIdx)
  const minColumn = Math.min(selection.anchor.colIdx, selection.active.colIdx)
  const maxColumn = Math.max(selection.anchor.colIdx, selection.active.colIdx)
  return (
    rowIndex >= minRow && rowIndex <= maxRow && columnIndex >= minColumn && columnIndex <= maxColumn
  )
}

export function isRowInRange(rowIndex: number, selection: CellSelection | null): boolean {
  if (!selection) return false
  const minRow = Math.min(selection.anchor.rowIdx, selection.active.rowIdx)
  const maxRow = Math.max(selection.anchor.rowIdx, selection.active.rowIdx)
  return rowIndex >= minRow && rowIndex <= maxRow
}

export function formatResultValue(value: ResultValue): {
  display: string
  isNull: boolean
  isNumeric: boolean
} {
  if (value.type === 'null') return { display: 'NULL', isNull: true, isNumeric: false }
  switch (value.type) {
    case 'text':
      return { display: value.text ?? '', isNull: false, isNumeric: false }
    case 'integer':
      return { display: String(value.integer ?? 0), isNull: false, isNumeric: true }
    case 'float':
      return { display: String(value.float ?? 0), isNull: false, isNumeric: true }
    case 'decimal':
      return { display: value.decimal ?? '', isNull: false, isNumeric: true }
    case 'bool':
      return { display: value.bool ? 'true' : 'false', isNull: false, isNumeric: false }
    case 'time':
      return { display: value.time ?? '', isNull: false, isNumeric: false }
    case 'bytes':
      return { display: '(binary)', isNull: false, isNumeric: false }
    default:
      return { display: '', isNull: false, isNumeric: false }
  }
}
