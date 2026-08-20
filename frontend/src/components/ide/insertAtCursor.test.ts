import { EditorState } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { describe, expect, it } from 'vitest'
import { insertAtCursor } from './insertAtCursor'

describe('insertAtCursor', () => {
  it('inserts text at the cursor position and moves the cursor after it', () => {
    const view = new EditorView({ state: EditorState.create({ doc: 'select 2 from foo;\n' }) })
    view.dispatch({ selection: { anchor: 8 } })

    insertAtCursor(view, 'select 1')

    expect(view.state.doc.toString()).toBe('select 2select 1 from foo;\n')
    expect(view.state.selection.main.head).toBe(8 + 'select 1'.length)
    view.destroy()
  })

  it('replaces a selection range with the inserted text', () => {
    const view = new EditorView({ state: EditorState.create({ doc: 'select 2 from foo;\n' }) })
    view.dispatch({ selection: { anchor: 7, head: 8 } })

    insertAtCursor(view, 'x')

    expect(view.state.doc.toString()).toBe('select x from foo;\n')
    view.destroy()
  })
})
