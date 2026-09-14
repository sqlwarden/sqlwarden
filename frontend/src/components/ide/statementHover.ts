import { ViewPlugin, type EditorView } from '@codemirror/view'
import { sqlStatementsWithOffsets, type SqlStatementWithOffsets } from './sqlStatements'

/** The statement whose range contains `offset`, or null if `offset` is null
 *  or falls in the gap between statements (unlike cursor tracking, hover
 *  should not fall back to a neighboring statement). The end bound is
 *  inclusive: the Run/Explain hint sits right after a statement's last
 *  character, and `posAtCoords` resolves the mouse to that exact offset
 *  once the pointer moves past the text — an exclusive bound would drop
 *  the hover right as the pointer reaches the hint. */
export function findHoveredStatement(
  statements: SqlStatementWithOffsets[],
  offset: number | null,
): SqlStatementWithOffsets | null {
  if (offset === null) return null
  return statements.find((s) => offset >= s.start && offset <= s.end) ?? null
}

/** A hovered statement's text, its offsets into the document (for previewing
 *  what will run), and the screen position (relative to the editor's DOM
 *  node) of the top-left corner of its own first character — where the
 *  Run/Explain hint anchors, bottom-aligned just above it. Anchoring to the
 *  statement's own start (not the shared line) keeps the hint correctly
 *  placed when multiple statements share one line. */
export type HoveredStatement = {
  sql: string
  start: number
  end: number
  top: number
  left: number
}

/** Reports the statement under the pointer as it moves over the editor
 *  content, or null when the pointer sits between statements. Coordinates
 *  are relative to `view.dom` so the caller can position an overlay hint
 *  without reaching into CodeMirror internals.
 *
 *  Deliberately has no `mouseleave` handler: the hint is rendered as a
 *  sibling overlay, not a CodeMirror decoration, so moving the pointer onto
 *  it crosses out of `view.dom` and would otherwise clear the hover state
 *  before a click can land. The caller clears hover state itself when the
 *  pointer leaves the wrapper that contains both the editor and the hint. */
export function statementHoverExtension(onHoverChange: (hovered: HoveredStatement | null) => void) {
  return ViewPlugin.define(() => ({}), {
    eventHandlers: {
      mousemove(event: MouseEvent, view: EditorView) {
        const offset = view.posAtCoords({ x: event.clientX, y: event.clientY })
        const statements = sqlStatementsWithOffsets(view.state.doc.toString())
        const hovered = findHoveredStatement(statements, offset)
        if (!hovered) {
          onHoverChange(null)
          return
        }
        const coords = view.coordsAtPos(hovered.start)
        if (!coords) {
          onHoverChange(null)
          return
        }
        const editorRect = view.dom.getBoundingClientRect()
        onHoverChange({
          sql: hovered.sql,
          start: hovered.start,
          end: hovered.end,
          top: coords.top - editorRect.top,
          left: coords.left - editorRect.left,
        })
      },
    },
  })
}
