import { EditorState } from '@codemirror/state'
import { closeSearchPanel, getSearchQuery, openSearchPanel, search } from '@codemirror/search'
import { EditorView } from '@codemirror/view'
import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'
import { FindPanel } from './FindPanel'

let view: EditorView | undefined

function createView(doc = 'alpha beta alpha', readOnly = false, customPanel = false) {
  const parent = document.createElement('div')
  document.body.appendChild(parent)
  view = new EditorView({
    parent,
    state: EditorState.create({
      doc,
      extensions: [
        search(
          customPanel ? { createPanel: () => ({ dom: document.createElement('div') }) } : undefined,
        ),
        EditorState.readOnly.of(readOnly),
      ],
    }),
  })
  return view
}

afterEach(() => {
  view?.destroy()
  view = undefined
})

describe('FindPanel', () => {
  it('updates search options and navigates matches from controls and the keyboard', async () => {
    const editor = createView('alpha beta alpha', false, true)
    const user = userEvent.setup()
    render(<FindPanel view={editor} />)

    const find = screen.getByPlaceholderText('Find')
    expect(find).toHaveFocus()
    await user.type(find, 'alpha')
    expect(screen.getByText('0/2')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Next match' }))
    expect(screen.getByText('1/2')).toBeInTheDocument()
    await user.keyboard('{Enter}')
    expect(screen.getByText('2/2')).toBeInTheDocument()
    await user.keyboard('{Shift>}{Enter}{/Shift}')
    expect(screen.getByText('1/2')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Match case' }))
    await user.click(screen.getByRole('button', { name: 'Use regular expression' }))
    await user.click(screen.getByRole('button', { name: 'Match whole word' }))
    expect(getSearchQuery(editor.state)).toEqual(
      expect.objectContaining({
        search: 'alpha',
        caseSensitive: true,
        regexp: true,
        wholeWord: true,
      }),
    )
  })

  it('replaces matches and closes the CodeMirror search panel', async () => {
    const editor = createView('alpha beta alpha', false, true)
    const user = userEvent.setup()
    act(() => {
      openSearchPanel(editor)
    })
    render(<FindPanel view={editor} />)

    await user.type(screen.getByPlaceholderText('Find'), 'alpha')
    await user.type(screen.getByPlaceholderText('Replace'), 'omega')
    await user.click(screen.getByRole('button', { name: 'Replace All' }))
    expect(editor.state.doc.toString()).toBe('omega beta omega')

    await user.click(screen.getByRole('button', { name: 'Close find' }))
    expect(closeSearchPanel(editor)).toBe(false)
  })

  it('does not expose replacement controls for read-only documents', () => {
    const editor = createView('alpha', true)
    render(<FindPanel view={editor} />)

    expect(screen.queryByPlaceholderText('Replace')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Replace All' })).not.toBeInTheDocument()
  })
})
