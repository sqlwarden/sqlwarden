import { createRef } from 'react'
import { openSearchPanel } from '@codemirror/search'
import { act, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import * as Y from 'yjs'
import { CsvRawEditor, type CsvRawViewState } from './CsvRawEditor'
import { EditorProviders, editorViewOf } from './rawEditorTestUtils'

function docWithContent(content: string): Y.Doc {
  const doc = new Y.Doc()
  doc.getText('content').insert(0, content)
  return doc
}

describe('CsvRawEditor', () => {
  it('shows the document text', () => {
    const doc = docWithContent('id,name\n1,Ada\n')
    const { container } = render(<CsvRawEditor doc={doc} viewState={createRef()} />, {
      wrapper: EditorProviders,
    })

    expect(editorViewOf(container).state.doc.toString()).toBe('id,name\n1,Ada\n')
  })

  it('writes edits back to the Y.Text', () => {
    const doc = docWithContent('id,name\n1,Ada\n')
    const { container } = render(<CsvRawEditor doc={doc} viewState={createRef()} />, {
      wrapper: EditorProviders,
    })

    act(() => {
      editorViewOf(container).dispatch({ changes: { from: 14, insert: '2,Grace\n' } })
    })

    expect(doc.getText('content').toString()).toBe('id,name\n1,Ada\n2,Grace\n')
  })

  it('reflects external Y.Text changes', () => {
    const doc = docWithContent('id\n')
    const { container } = render(<CsvRawEditor doc={doc} viewState={createRef()} />, {
      wrapper: EditorProviders,
    })

    act(() => {
      doc.getText('content').insert(3, '1\n')
    })

    expect(editorViewOf(container).state.doc.toString()).toBe('id\n1\n')
  })

  it('keeps the cursor across a remount through the shared view state', () => {
    const doc = docWithContent('id,name\n1,Ada\n')
    const viewState = { current: null as CsvRawViewState | null }
    const first = render(<CsvRawEditor doc={doc} viewState={viewState} />, {
      wrapper: EditorProviders,
    })
    act(() => {
      editorViewOf(first.container).dispatch({ selection: { anchor: 5 } })
    })
    first.unmount()

    const second = render(<CsvRawEditor doc={doc} viewState={viewState} />, {
      wrapper: EditorProviders,
    })

    expect(editorViewOf(second.container).state.selection.main.anchor).toBe(5)
  })

  it('opens the app-styled find and replace panel', async () => {
    const doc = docWithContent('id,name\n1,Ada\n')
    const { container } = render(<CsvRawEditor doc={doc} viewState={createRef()} />, {
      wrapper: EditorProviders,
    })

    await act(async () => {
      openSearchPanel(editorViewOf(container))
      await Promise.resolve()
    })

    expect(await screen.findByPlaceholderText('Find')).toBeInTheDocument()
    expect(container.querySelector('.cm-sqlwarden-find')).not.toBeNull()
  })
})
