const MIN_NAME_COL_WIDTH = 60
const MAX_NAME_COL_WIDTH = 320
const NAME_CHAR_WIDTH_PX = 7
const NAME_COL_PADDING_PX = 56

/** Sizes a column from its name length (icon + type row need room too) so
 *  columns start close to their content width instead of a flat default. */
export function columnWidthFromName(name: string): number {
  const estimate = name.length * NAME_CHAR_WIDTH_PX + NAME_COL_PADDING_PX
  return Math.min(MAX_NAME_COL_WIDTH, Math.max(MIN_NAME_COL_WIDTH, estimate))
}

export type ColumnWidthDistribution = {
  /** Rendered width per column: auto (never manually resized) columns grow
   *  to share out any width left over once the grid is narrower than its
   *  container; manually resized columns keep their exact width. */
  displayWidths: number[]
  /** Whether each column grew beyond its stored width this render. */
  columnGrew: boolean[]
}

/** Distributes any leftover container width across columns the user hasn't
 *  manually resized, so an N-column result fills the pane instead of
 *  leaving a gap after the last column. A column counts as "auto" as long
 *  as its stored width still matches its name-derived default. */
export function distributeColumnWidths(
  columnWidths: number[],
  defaultWidths: number[],
  rowNumColWidth: number,
  containerWidth: number,
): ColumnWidthDistribution {
  const isAuto = columnWidths.map((w, i) => w === defaultWidths[i])
  const growableCount = isAuto.filter(Boolean).length
  const usedWidth = rowNumColWidth + columnWidths.reduce((a, b) => a + b, 0)
  const extraPerColumn =
    growableCount > 0 && containerWidth > usedWidth
      ? (containerWidth - usedWidth) / growableCount
      : 0
  return {
    displayWidths: columnWidths.map((w, i) => (isAuto[i] ? w + extraPerColumn : w)),
    columnGrew: isAuto.map((auto) => auto && extraPerColumn > 0),
  }
}
