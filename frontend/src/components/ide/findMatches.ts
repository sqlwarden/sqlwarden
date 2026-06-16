import type { EditorState } from '@codemirror/state'
import type { SearchQuery } from '@codemirror/search'

export type MatchInfo = {
  /** 1-based index of the currently selected match, or 0 if none is selected. */
  current: number
  /** Total number of matches in the document. */
  total: number
}

/**
 * Counts the matches of `query` in `state` and, when the main selection exactly
 * covers one of them, reports its 1-based position. Used to render the "N / M"
 * match counter in the find panel.
 */
export function computeMatchInfo(state: EditorState, query: SearchQuery): MatchInfo {
  if (!query.valid) return { current: 0, total: 0 }

  const sel = state.selection.main
  let total = 0
  let current = 0

  const cursor = query.getCursor(state)
  for (let next = cursor.next(); !next.done; next = cursor.next()) {
    total++
    if (next.value.from === sel.from && next.value.to === sel.to) {
      current = total
    }
  }

  return { current, total }
}
