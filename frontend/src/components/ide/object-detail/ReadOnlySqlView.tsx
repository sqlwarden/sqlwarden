import { useEffect, useRef } from 'react'
import { EditorView, lineNumbers, highlightSpecialChars } from '@codemirror/view'
import { EditorState, Compartment, type Extension } from '@codemirror/state'
import { syntaxHighlighting, defaultHighlightStyle } from '@codemirror/language'
import { sql } from '@codemirror/lang-sql'
import { cn } from '#/lib/utils'
import { useTheme } from '#/components/theme-provider'
import { useEditorTheme } from '#/lib/editor-themes/context'
import { loadEditorTheme, getCachedTheme } from '#/lib/editor-themes'
import { useEditorFont, loadEditorFont } from '#/lib/editor-font/context'

function baseTheme(fontFamily: string, fontSize: number): Extension {
  return EditorView.theme({
    '&': { height: '100%' },
    '.cm-scroller': { fontFamily, fontSize: `${fontSize}px`, lineHeight: '1.65', overflow: 'auto' },
    '.cm-content': { padding: '8px 0' },
    '.cm-lineNumbers .cm-gutterElement': { minWidth: '3.5ch', textAlign: 'right' },
  })
}

/** A read-only SQL viewer that reuses the user's editor theme/font preferences
 *  and SQL syntax highlighting, so a DDL/definition reads like the editor. */
export function ReadOnlySqlView({ value, className }: { value: string; className?: string }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const themeCompartment = useRef(new Compartment())
  const fontCompartment = useRef(new Compartment())

  const { resolvedTheme } = useTheme()
  const { editorThemeDark, editorThemeLight } = useEditorTheme()
  const activeThemeName = resolvedTheme === 'dark' ? editorThemeDark : editorThemeLight
  const { editorFont, editorFontSize } = useEditorFont()

  // Create the view once; doc/theme/font are updated via dispatch below.
  useEffect(() => {
    if (!containerRef.current) return
    const view = new EditorView({
      state: EditorState.create({
        doc: value,
        extensions: [
          lineNumbers(),
          highlightSpecialChars(),
          syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
          sql(),
          EditorView.lineWrapping,
          EditorView.editable.of(false),
          EditorState.readOnly.of(true),
          fontCompartment.current.of(baseTheme(editorFont.fontFamily, editorFontSize)),
          themeCompartment.current.of(getCachedTheme(activeThemeName) ?? []),
        ],
      }),
      parent: containerRef.current,
    })
    viewRef.current = view
    return () => {
      viewRef.current = null
      view.destroy()
    }
    // Created once; value/theme/font are applied by the effects below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Replace the document when the source text changes.
  useEffect(() => {
    const view = viewRef.current
    if (!view || value === view.state.doc.toString()) return
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: value } })
  }, [value])

  // Hot-swap theme.
  useEffect(() => {
    let cancelled = false
    loadEditorTheme(activeThemeName).then((ext) => {
      if (cancelled || !viewRef.current) return
      viewRef.current.dispatch({ effects: themeCompartment.current.reconfigure(ext) })
    })
    return () => { cancelled = true }
  }, [activeThemeName])

  // Hot-swap font / size.
  useEffect(() => {
    let cancelled = false
    loadEditorFont(editorFont).then(() => {
      if (cancelled || !viewRef.current) return
      viewRef.current.dispatch({ effects: fontCompartment.current.reconfigure(baseTheme(editorFont.fontFamily, editorFontSize)) })
    })
    return () => { cancelled = true }
  }, [editorFont, editorFontSize])

  return <div ref={containerRef} className={cn('h-full overflow-hidden', className)} />
}
