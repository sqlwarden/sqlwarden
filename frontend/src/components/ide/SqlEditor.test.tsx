import type { PropsWithChildren } from 'react'
import { openSearchPanel } from '@codemirror/search'
import { act, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import * as Y from 'yjs'
import { ThemeProvider } from '#/components/theme-provider'
import { EditorFontProvider } from '#/lib/editor-font/context'
import { EditorThemeProvider } from '#/lib/editor-themes/context'
import { SqlEditor } from './SqlEditor'
import { createEditorViewRegistry, EditorViewRegistryContext } from './useEditorViewRegistry'
import { createIdeStore, IdeStoreContext } from './useIdeStore'

describe('SqlEditor', () => {
  it('owns the CodeMirror and Y.Doc lifecycle for a pane', async () => {
    const registry = createEditorViewRegistry()
    const store = createIdeStore('acme', 1, 'ephemeral')
    const doc = new Y.Doc()
    doc.getText('content').insert(0, 'select 1')
    const onCursorChange = vi.fn()

    function Providers({ children }: PropsWithChildren) {
      return (
        <ThemeProvider defaultTheme="light" disableTransitionOnChange={false}>
          <EditorThemeProvider>
            <EditorFontProvider>
              <IdeStoreContext.Provider value={store}>
                <EditorViewRegistryContext.Provider value={registry}>
                  {children}
                </EditorViewRegistryContext.Provider>
              </IdeStoreContext.Provider>
            </EditorFontProvider>
          </EditorThemeProvider>
        </ThemeProvider>
      )
    }

    const rendered = render(
      <SqlEditor tabId="query" groupId="left" doc={doc} onCursorChange={onCursorChange} />,
      { wrapper: Providers },
    )

    const editor = await waitFor(() => {
      const registered = registry.get('left:query')
      expect(registered).toBeDefined()
      return registered!
    })
    await act(async () => {
      await Promise.resolve()
    })
    expect(editor.state.doc.toString()).toBe('select 1')

    act(() => {
      editor.dispatch({ selection: { anchor: 8 } })
    })
    expect(onCursorChange).toHaveBeenLastCalledWith(1, 9, 0)

    act(() => {
      editor.dispatch({ changes: { from: 0, to: 6, insert: 'SELECT' } })
    })
    expect(doc.getText('content').toString()).toBe('SELECT 1')

    await act(async () => {
      openSearchPanel(editor)
      await Promise.resolve()
    })
    expect(await screen.findByPlaceholderText('Find')).toBeInTheDocument()

    const focus = vi.spyOn(editor, 'focus')
    await act(async () => {
      store.getState().setFocusEditorRequest('left:query')
      await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()))
    })
    expect(focus).toHaveBeenCalled()
    expect(store.getState().focusEditorRequest).toBeNull()

    act(() => rendered.unmount())
    expect(registry.get('left:query')).toBeUndefined()
    doc.destroy()
  })

  it('applies a pending line/column jump from search and clears it', async () => {
    const registry = createEditorViewRegistry()
    const store = createIdeStore('acme', 1, 'ephemeral')
    const doc = new Y.Doc()
    doc.getText('content').insert(0, 'select 1\nselect 2\nselect 3')

    function Providers({ children }: PropsWithChildren) {
      return (
        <ThemeProvider defaultTheme="light" disableTransitionOnChange={false}>
          <EditorThemeProvider>
            <EditorFontProvider>
              <IdeStoreContext.Provider value={store}>
                <EditorViewRegistryContext.Provider value={registry}>
                  {children}
                </EditorViewRegistryContext.Provider>
              </IdeStoreContext.Provider>
            </EditorFontProvider>
          </EditorThemeProvider>
        </ThemeProvider>
      )
    }

    const rendered = render(<SqlEditor tabId="file:9" groupId="left" doc={doc} />, {
      wrapper: Providers,
    })

    const editor = await waitFor(() => {
      const registered = registry.get('left:file:9')
      expect(registered).toBeDefined()
      return registered!
    })

    const focus = vi.spyOn(editor, 'focus')
    act(() => {
      store.getState().setPendingJump({ tabId: 'file:9', line: 2, column: 4 })
    })

    await waitFor(() => {
      expect(editor.state.selection.main.head).toBe(editor.state.doc.line(2).from + 3)
    })
    expect(focus).toHaveBeenCalled()
    expect(store.getState().pendingJump).toBeNull()

    act(() => rendered.unmount())
    doc.destroy()
  })
})
