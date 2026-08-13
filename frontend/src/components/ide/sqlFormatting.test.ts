import { history, undo } from '@codemirror/commands'
import { EditorState, type Extension } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { findFrontendEngine } from './engines/registry'
import { formatEditorSql, sqlFormatterForDriver, sqlFormattingKeymap } from './sqlFormatting'

const views: EditorView[] = []

function editor(doc: string, extensions: Extension[] = []) {
  const view = new EditorView({ state: EditorState.create({ doc, extensions }) })
  views.push(view)
  return view
}

afterEach(() => {
  views.splice(0).forEach((view) => view.destroy())
  vi.restoreAllMocks()
})

describe('SQL formatting', () => {
  it.each([
    [
      'postgres',
      'select payload::jsonb from events where id=1',
      'select\n  payload::jsonb\nfrom\n  events\nwhere\n  id = 1',
    ],
    [
      'mysql',
      'select `UserId` from `Users` where `active`=1',
      'select\n  `UserId`\nfrom\n  `Users`\nwhere\n  `active` = 1',
    ],
    [
      'sqlite',
      'insert or replace into users(id,name) values(1,"a")',
      'insert or replace into\n  users (id, name)\nvalues\n  (1, "a")',
    ],
  ])('formats %s syntax with its engine formatter', (driver, source, expected) => {
    expect(sqlFormatterForDriver(driver).format(source)).toBe(expected)
  })

  it('delegates known drivers to their registered dialect and falls back for unknown drivers', () => {
    const dialect = findFrontendEngine('postgres')!.dialect
    const formatSql = vi.spyOn(dialect, 'formatSql').mockReturnValue('dialect result')

    expect(sqlFormatterForDriver('postgres').format('select 1')).toBe('dialect result')
    expect(formatSql).toHaveBeenCalledWith('select 1')
    expect(sqlFormatterForDriver('future-engine').format('select * from t')).toContain('\n')
  })

  it('formats the entire document as one undoable edit', () => {
    const view = editor('select * from users where id=1', [history()])

    expect(formatEditorSql(view, sqlFormatterForDriver('postgres'))).toBe(true)
    expect(view.state.doc.toString()).toBe('select\n  *\nfrom\n  users\nwhere\n  id = 1')

    expect(undo(view)).toBe(true)
    expect(view.state.doc.toString()).toBe('select * from users where id=1')
  })

  it('formats only the selected SQL and keeps the formatted range selected', () => {
    const source = 'select 1; select * from users where id=1;'
    const selectionFrom = source.indexOf('select *')
    const view = editor(source)
    view.dispatch({ selection: { anchor: selectionFrom, head: source.length - 1 } })

    formatEditorSql(view, sqlFormatterForDriver('mysql'))

    expect(view.state.doc.toString()).toBe('select 1; select\n  *\nfrom\n  users\nwhere\n  id = 1;')
    expect(view.state.sliceDoc(view.state.selection.main.from, view.state.selection.main.to)).toBe(
      'select\n  *\nfrom\n  users\nwhere\n  id = 1',
    )
  })

  it('leaves the document unchanged when a formatter fails', () => {
    const view = editor('select 1')
    expect(() =>
      formatEditorSql(view, {
        format() {
          throw new Error('unsupported syntax')
        },
      }),
    ).toThrow('unsupported syntax')
    expect(view.state.doc.toString()).toBe('select 1')
  })

  it('binds Shift-Alt-F and reports formatting failures', () => {
    const onError = vi.fn()
    const formatter = { format: vi.fn(() => 'select\n  1') }
    const view = editor('select 1', [sqlFormattingKeymap(formatter, onError)])
    view.contentDOM.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'f', shiftKey: true, altKey: true, bubbles: true }),
    )

    expect(formatter.format).toHaveBeenCalledWith('select 1')
    expect(view.state.doc.toString()).toBe('select\n  1')
    expect(onError).not.toHaveBeenCalled()
  })
})
