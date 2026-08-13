import { EditorSelection, Transaction } from '@codemirror/state'
import { keymap, type EditorView, type KeyBinding } from '@codemirror/view'
import { findFrontendEngine } from './engines/registry'
import { defaultSqlFormatter, type SqlTextFormatter } from './sqlFormatter'

export const FORMAT_SQL_KEY = 'Shift-Alt-f'

export function sqlFormatterForDriver(driver?: string): SqlTextFormatter {
  if (!driver) return defaultSqlFormatter
  const dialect = findFrontendEngine(driver)?.dialect
  return dialect ? { format: (sql) => dialect.formatSql(sql) } : defaultSqlFormatter
}

/** Formats the selected SQL, or the whole document when the selection is empty. */
export function formatEditorSql(view: EditorView, formatter: SqlTextFormatter): boolean {
  const selection = view.state.selection.main
  const from = selection.empty ? 0 : selection.from
  const to = selection.empty ? view.state.doc.length : selection.to
  const source = view.state.doc.sliceString(from, to)
  if (source.trim() === '') {
    view.focus()
    return true
  }

  const formatted = formatter.format(source)
  if (formatted === source) {
    view.focus()
    return true
  }

  const nextSelection = selection.empty
    ? EditorSelection.cursor(Math.min(selection.head, formatted.length))
    : EditorSelection.range(from, from + formatted.length)

  view.dispatch({
    changes: { from, to, insert: formatted },
    selection: nextSelection,
    annotations: Transaction.userEvent.of('input.format'),
    scrollIntoView: true,
  })
  view.focus()
  return true
}

export function sqlFormattingKeymap(
  formatter: SqlTextFormatter,
  onError: (error: unknown) => void,
): ReturnType<typeof keymap.of> {
  const bindings: KeyBinding[] = [
    {
      key: FORMAT_SQL_KEY,
      run(view) {
        try {
          return formatEditorSql(view, formatter)
        } catch (error) {
          onError(error)
          return true
        }
      },
    },
  ]
  return keymap.of(bindings)
}
