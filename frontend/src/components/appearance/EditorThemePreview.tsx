import { useEffect, useRef } from 'react'
import { EditorState, Compartment, type Extension } from '@codemirror/state'
import { EditorView, lineNumbers } from '@codemirror/view'
import { syntaxHighlighting, defaultHighlightStyle } from '@codemirror/language'
import { sql, PostgreSQL } from '@codemirror/lang-sql'
import { cn } from '#/lib/utils'
import { useTheme } from '#/components/theme-provider'
import { useEditorTheme } from '#/lib/editor-themes/context'
import { loadEditorTheme, getCachedTheme } from '#/lib/editor-themes'
import { useEditorFont, editorFontSizeRem } from '#/lib/editor-font/context'
import type { EditorFont, EditorFontSize } from '#/lib/editor-font/context'
import type { EditorThemeName } from '#/lib/editor-themes'
import { SAMPLE_QUERY } from './sampleQuery'

function baseTheme(font: EditorFont, size: EditorFontSize): Extension {
  return EditorView.theme({
    '&': { height: '100%' },
    '.cm-scroller': {
      fontFamily: font.fontFamily,
      fontSize: editorFontSizeRem(size),
      lineHeight: '1.65',
      overflow: 'auto',
    },
    '.cm-content': { padding: '8px 0' },
    '.cm-gutters': { border: 'none' },
    '.cm-lineNumbers .cm-gutterElement': { minWidth: '3ch', textAlign: 'right' },
  })
}

export function EditorThemePreview({ className }: { className?: string }) {
  const hostRef = useRef<HTMLDivElement>(null)
  const viewRef = useRef<EditorView | null>(null)
  const themeComp = useRef(new Compartment())
  const fontComp = useRef(new Compartment())

  const { resolvedTheme } = useTheme()
  const { editorThemeDark, editorThemeLight } = useEditorTheme()
  const { editorFont, editorFontSize } = useEditorFont()
  const activeThemeName: EditorThemeName =
    resolvedTheme === 'dark' ? editorThemeDark : editorThemeLight

  // Mount once.
  useEffect(() => {
    if (!hostRef.current) return
    const view = new EditorView({
      parent: hostRef.current,
      state: EditorState.create({
        doc: SAMPLE_QUERY,
        extensions: [
          EditorState.readOnly.of(true),
          EditorView.editable.of(false),
          lineNumbers(),
          // Match the live editor: the default style is only a fallback, so
          // each editor theme's own HighlightStyle drives the token colors.
          syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
          sql({ dialect: PostgreSQL, upperCaseKeywords: true }),
          themeComp.current.of(getCachedTheme(activeThemeName) ?? []),
          fontComp.current.of(baseTheme(editorFont, editorFontSize)),
        ],
      }),
    })
    viewRef.current = view
    return () => {
      view.destroy()
      viewRef.current = null
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- mount-only; live updates handled by the effects below
  }, [])

  // Swap the CodeMirror theme when the resolved app theme or the chosen editor theme changes.
  useEffect(() => {
    let cancelled = false
    void loadEditorTheme(activeThemeName).then((ext) => {
      if (cancelled || !viewRef.current) return
      viewRef.current.dispatch({ effects: themeComp.current.reconfigure(ext) })
    })
    return () => {
      cancelled = true
    }
  }, [activeThemeName])

  // Re-apply font family / size.
  useEffect(() => {
    viewRef.current?.dispatch({
      effects: fontComp.current.reconfigure(baseTheme(editorFont, editorFontSize)),
    })
  }, [editorFont, editorFontSize])

  return (
    <div
      ref={hostRef}
      role="img"
      aria-label="Editor theme preview"
      className={cn(
        'h-44 overflow-hidden rounded-md border border-border bg-background text-left',
        className,
      )}
    />
  )
}
