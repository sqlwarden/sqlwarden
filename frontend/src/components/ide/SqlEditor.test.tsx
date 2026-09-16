import type { PropsWithChildren } from 'react'
import { openSearchPanel } from '@codemirror/search'
import { EditorView } from '@codemirror/view'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import * as Y from 'yjs'
import { ThemeProvider } from '#/components/theme-provider'
import { ContextMenuProvider } from '#/components/ui/context-menu'
import { EditorFontProvider } from '#/lib/editor-font/context'
import { EditorThemeProvider } from '#/lib/editor-themes/context'
import { SqlEditor } from './SqlEditor'
import { createEditorViewRegistry, EditorViewRegistryContext } from './useEditorViewRegistry'
import { createTabViewStateCache, TabViewStateCacheContext } from './tabViewStateCache'
import { createIdeStore, IdeStoreContext } from './useIdeStore'

describe('SqlEditor', () => {
  it('owns the CodeMirror and Y.Doc lifecycle for a pane', async () => {
    const registry = createEditorViewRegistry()
    const tabViewStateCache = createTabViewStateCache()
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
                  <TabViewStateCacheContext.Provider value={tabViewStateCache}>
                    {children}
                  </TabViewStateCacheContext.Provider>
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
    expect(onCursorChange).toHaveBeenLastCalledWith(1, 9, 0, '')

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

  it('restores scroll position and selection when a tab remounts after being switched away from', async () => {
    const registry = createEditorViewRegistry()
    const tabViewStateCache = createTabViewStateCache()
    const store = createIdeStore('acme', 1, 'ephemeral')
    const scrollSnapshotSpy = vi.spyOn(EditorView.prototype, 'scrollSnapshot')
    const doc = new Y.Doc()
    doc
      .getText('content')
      .insert(0, Array.from({ length: 200 }, (_, i) => `select ${i};`).join('\n'))

    function Providers({ children }: PropsWithChildren) {
      return (
        <ThemeProvider defaultTheme="light" disableTransitionOnChange={false}>
          <EditorThemeProvider>
            <EditorFontProvider>
              <IdeStoreContext.Provider value={store}>
                <EditorViewRegistryContext.Provider value={registry}>
                  <TabViewStateCacheContext.Provider value={tabViewStateCache}>
                    {children}
                  </TabViewStateCacheContext.Provider>
                </EditorViewRegistryContext.Provider>
              </IdeStoreContext.Provider>
            </EditorFontProvider>
          </EditorThemeProvider>
        </ThemeProvider>
      )
    }

    const rendered = render(<SqlEditor tabId="query" groupId="left" doc={doc} />, {
      wrapper: Providers,
    })

    const firstEditor = await waitFor(() => {
      const registered = registry.get('left:query')
      expect(registered).toBeDefined()
      return registered!
    })

    act(() => {
      firstEditor.dispatch({ selection: { anchor: 20, head: 30 } })
    })

    act(() => rendered.unmount())
    expect(registry.get('left:query')).toBeUndefined()
    expect(scrollSnapshotSpy).toHaveBeenCalledTimes(1)
    const savedScroll = tabViewStateCache.load('left:query')?.scroll
    expect(savedScroll).toBeDefined()
    expect(scrollSnapshotSpy.mock.results[0]?.value).toBe(savedScroll)

    render(<SqlEditor tabId="query" groupId="left" doc={doc} />, { wrapper: Providers })

    const secondEditor = await waitFor(() => {
      const registered = registry.get('left:query')
      expect(registered).toBeDefined()
      return registered!
    })

    expect(secondEditor.state.selection.main.anchor).toBe(20)
    expect(secondEditor.state.selection.main.head).toBe(30)

    doc.destroy()
  })

  it('passes the selected text to onCursorChange', async () => {
    const registry = createEditorViewRegistry()
    const tabViewStateCache = createTabViewStateCache()
    const store = createIdeStore('acme', 1, 'ephemeral')
    const doc = new Y.Doc()
    doc.getText('content').insert(0, 'select 1;\nselect 2;')
    const onCursorChange = vi.fn()

    function Providers({ children }: PropsWithChildren) {
      return (
        <ThemeProvider defaultTheme="light" disableTransitionOnChange={false}>
          <EditorThemeProvider>
            <EditorFontProvider>
              <IdeStoreContext.Provider value={store}>
                <EditorViewRegistryContext.Provider value={registry}>
                  <TabViewStateCacheContext.Provider value={tabViewStateCache}>
                    {children}
                  </TabViewStateCacheContext.Provider>
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

    act(() => {
      editor.dispatch({ selection: { anchor: 0, head: 9 } })
    })
    expect(onCursorChange).toHaveBeenLastCalledWith(1, 10, 9, 'select 1;')

    act(() => rendered.unmount())
    doc.destroy()
  })

  it('applies a pending line/column jump from search and clears it', async () => {
    const registry = createEditorViewRegistry()
    const tabViewStateCache = createTabViewStateCache()
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
                  <TabViewStateCacheContext.Provider value={tabViewStateCache}>
                    {children}
                  </TabViewStateCacheContext.Provider>
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

  it('right-click opens a context menu that differs by tab type', async () => {
    const registry = createEditorViewRegistry()
    const tabViewStateCache = createTabViewStateCache()
    const store = createIdeStore('acme', 1, 'ephemeral')
    const doc = new Y.Doc()
    doc.getText('content').insert(0, 'select 1')
    const onRunStatement = vi.fn()
    const onFormat = vi.fn()

    function Providers({ children }: PropsWithChildren) {
      return (
        <ThemeProvider defaultTheme="light" disableTransitionOnChange={false}>
          <EditorThemeProvider>
            <EditorFontProvider>
              <IdeStoreContext.Provider value={store}>
                <EditorViewRegistryContext.Provider value={registry}>
                  <TabViewStateCacheContext.Provider value={tabViewStateCache}>
                    <ContextMenuProvider>{children}</ContextMenuProvider>
                  </TabViewStateCacheContext.Provider>
                </EditorViewRegistryContext.Provider>
              </IdeStoreContext.Provider>
            </EditorFontProvider>
          </EditorThemeProvider>
        </ThemeProvider>
      )
    }

    const rendered = render(
      <SqlEditor
        tabId="query"
        groupId="left"
        doc={doc}
        contextMenu={{
          isSqlTab: true,
          canRun: true,
          onRunStatement,
          onRunAll: vi.fn(),
          canExplain: true,
          canExplainAnalyze: true,
          onExplain: vi.fn(),
          onExplainAnalyze: vi.fn(),
          onFormat,
          onSaveFavorite: vi.fn(),
          onRunSegment: vi.fn(),
          onExplainSegment: vi.fn(),
        }}
      />,
      { wrapper: Providers },
    )

    await waitFor(() => expect(registry.get('left:query')).toBeDefined())

    const cmContent = rendered.container.querySelector('.cm-content')
    expect(cmContent).toBeTruthy()
    fireEvent.contextMenu(cmContent!)
    expect(await screen.findByText('Run Statement')).toBeInTheDocument()

    fireEvent.click(screen.getByText('Format SQL'))
    expect(onFormat).toHaveBeenCalled()

    doc.destroy()
  })

  it('shows the Run/Explain hint only while text is selected, runs exactly the selection, and positions by selection direction', async () => {
    const registry = createEditorViewRegistry()
    const tabViewStateCache = createTabViewStateCache()
    const store = createIdeStore('acme', 1, 'ephemeral')
    const doc = new Y.Doc()
    doc.getText('content').insert(0, 'select 1;\nselect 2;')
    const onRunSegment = vi.fn()

    function Providers({ children }: PropsWithChildren) {
      return (
        <ThemeProvider defaultTheme="light" disableTransitionOnChange={false}>
          <EditorThemeProvider>
            <EditorFontProvider>
              <IdeStoreContext.Provider value={store}>
                <EditorViewRegistryContext.Provider value={registry}>
                  <TabViewStateCacheContext.Provider value={tabViewStateCache}>
                    {children}
                  </TabViewStateCacheContext.Provider>
                </EditorViewRegistryContext.Provider>
              </IdeStoreContext.Provider>
            </EditorFontProvider>
          </EditorThemeProvider>
        </ThemeProvider>
      )
    }

    render(
      <SqlEditor
        tabId="query"
        groupId="left"
        doc={doc}
        contextMenu={{
          isSqlTab: true,
          canRun: true,
          onRunStatement: vi.fn(),
          onRunAll: vi.fn(),
          canExplain: true,
          canExplainAnalyze: true,
          onExplain: vi.fn(),
          onExplainAnalyze: vi.fn(),
          onFormat: vi.fn(),
          onSaveFavorite: vi.fn(),
          onRunSegment,
          onExplainSegment: vi.fn(),
        }}
      />,
      { wrapper: Providers },
    )

    const editor = await waitFor(() => {
      const registered = registry.get('left:query')
      expect(registered).toBeDefined()
      return registered!
    })

    act(() => {
      editor.dispatch({ selection: { anchor: 3 } })
    })
    expect(screen.queryByText('Run')).not.toBeInTheDocument()

    act(() => {
      editor.dispatch({ selection: { anchor: 0, head: 8 } })
    })
    const runButton = await screen.findByText('Run')
    expect(runButton.parentElement).not.toHaveClass('-translate-y-full')

    fireEvent.click(runButton)
    expect(onRunSegment).toHaveBeenCalledWith('select 1')
    expect(screen.queryByText('Run')).not.toBeInTheDocument()

    act(() => {
      editor.dispatch({ selection: { anchor: 8, head: 0 } })
    })
    const runButtonBackward = await screen.findByText('Run')
    expect(runButtonBackward.parentElement).toHaveClass('-translate-y-full')

    act(() => {
      editor.dispatch({ selection: { anchor: 0 } })
    })
    await waitFor(() => expect(screen.queryByText('Run')).not.toBeInTheDocument())

    doc.destroy()
  })

  it('withholds the Run/Explain hint while the mouse button is held during a drag-selection, showing it only on mouseup', async () => {
    const registry = createEditorViewRegistry()
    const tabViewStateCache = createTabViewStateCache()
    const store = createIdeStore('acme', 1, 'ephemeral')
    const doc = new Y.Doc()
    doc.getText('content').insert(0, 'select 1;\nselect 2;')

    function Providers({ children }: PropsWithChildren) {
      return (
        <ThemeProvider defaultTheme="light" disableTransitionOnChange={false}>
          <EditorThemeProvider>
            <EditorFontProvider>
              <IdeStoreContext.Provider value={store}>
                <EditorViewRegistryContext.Provider value={registry}>
                  <TabViewStateCacheContext.Provider value={tabViewStateCache}>
                    {children}
                  </TabViewStateCacheContext.Provider>
                </EditorViewRegistryContext.Provider>
              </IdeStoreContext.Provider>
            </EditorFontProvider>
          </EditorThemeProvider>
        </ThemeProvider>
      )
    }

    render(
      <SqlEditor
        tabId="query"
        groupId="left"
        doc={doc}
        contextMenu={{
          isSqlTab: true,
          canRun: true,
          onRunStatement: vi.fn(),
          onRunAll: vi.fn(),
          canExplain: true,
          canExplainAnalyze: true,
          onExplain: vi.fn(),
          onExplainAnalyze: vi.fn(),
          onFormat: vi.fn(),
          onSaveFavorite: vi.fn(),
          onRunSegment: vi.fn(),
          onExplainSegment: vi.fn(),
        }}
      />,
      { wrapper: Providers },
    )

    const editor = await waitFor(() => {
      const registered = registry.get('left:query')
      expect(registered).toBeDefined()
      return registered!
    })

    fireEvent.mouseDown(editor.contentDOM, { button: 0 })
    act(() => {
      editor.dispatch({ selection: { anchor: 0, head: 4 } })
    })
    expect(screen.queryByText('Run')).not.toBeInTheDocument()

    act(() => {
      editor.dispatch({ selection: { anchor: 0, head: 8 } })
    })
    expect(screen.queryByText('Run')).not.toBeInTheDocument()

    fireEvent.mouseUp(window)
    expect(await screen.findByText('Run')).toBeInTheDocument()

    doc.destroy()
  })

  it('omits the run/format section when the tab is not SQL', async () => {
    const registry = createEditorViewRegistry()
    const tabViewStateCache = createTabViewStateCache()
    const store = createIdeStore('acme', 1, 'ephemeral')
    const doc = new Y.Doc()
    doc.getText('content').insert(0, 'not sql')

    function Providers({ children }: PropsWithChildren) {
      return (
        <ThemeProvider defaultTheme="light" disableTransitionOnChange={false}>
          <EditorThemeProvider>
            <EditorFontProvider>
              <IdeStoreContext.Provider value={store}>
                <EditorViewRegistryContext.Provider value={registry}>
                  <TabViewStateCacheContext.Provider value={tabViewStateCache}>
                    <ContextMenuProvider>{children}</ContextMenuProvider>
                  </TabViewStateCacheContext.Provider>
                </EditorViewRegistryContext.Provider>
              </IdeStoreContext.Provider>
            </EditorFontProvider>
          </EditorThemeProvider>
        </ThemeProvider>
      )
    }

    const rendered = render(<SqlEditor tabId="query" groupId="left" doc={doc} />, {
      wrapper: Providers,
    })

    await waitFor(() => expect(registry.get('left:query')).toBeDefined())

    const cmContent = rendered.container.querySelector('.cm-content')
    expect(cmContent).toBeTruthy()
    fireEvent.contextMenu(cmContent!)
    expect(await screen.findByText('Copy')).toBeInTheDocument()
    expect(screen.queryByText('Run Statement')).not.toBeInTheDocument()

    doc.destroy()
  })
})
