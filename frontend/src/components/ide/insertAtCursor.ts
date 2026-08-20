import { EditorSelection, Transaction } from '@codemirror/state'
import type { EditorView } from '@codemirror/view'

export function insertAtCursor(view: EditorView, text: string) {
  const { from, to } = view.state.selection.main
  view.dispatch({
    changes: { from, to, insert: text },
    selection: EditorSelection.cursor(from + text.length),
    annotations: Transaction.userEvent.of('input.paste'),
    scrollIntoView: true,
  })
  view.focus()
}
