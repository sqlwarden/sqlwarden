export type GridCell = { rowIdx: number; colIdx: number }

/** Next selected cell for an arrow key, clamped to the grid. null for other keys. */
export function nextCell(
  key: string,
  current: GridCell,
  rowCount: number,
  colCount: number,
): GridCell | null {
  const { rowIdx, colIdx } = current
  switch (key) {
    case 'ArrowUp':
      return { rowIdx: Math.max(0, rowIdx - 1), colIdx }
    case 'ArrowDown':
      return { rowIdx: Math.min(rowCount - 1, rowIdx + 1), colIdx }
    case 'ArrowLeft':
      return { rowIdx, colIdx: Math.max(0, colIdx - 1) }
    case 'ArrowRight':
      return { rowIdx, colIdx: Math.min(colCount - 1, colIdx + 1) }
    default:
      return null
  }
}
