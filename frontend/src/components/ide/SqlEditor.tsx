import { useEffect, useRef } from 'react'
import { EditorView, basicSetup } from 'codemirror'
import { EditorState, Compartment } from '@codemirror/state'
import { sql } from '@codemirror/lang-sql'
import { yCollab } from 'y-codemirror.next'
import type * as Y from 'yjs'
import { cn } from '#/lib/utils'
import { useTheme } from '#/components/theme-provider'
import { useEditorTheme } from '#/lib/editor-themes/context'
import { loadEditorTheme, getCachedTheme } from '#/lib/editor-themes'
import { useEditorViewRegistry } from './useEditorViewRegistry'

// ─── Base structural theme ──────────────────────────────────────────────────────
// Layout and font styles that must always be present regardless of which
// color theme is loaded in the Compartment. The @uiw themes do not set these.

const baseEditorTheme = EditorView.theme({
  '&': { height: '100%' },
  '.cm-scroller': {
    fontFamily: 'var(--font-mono, ui-monospace, monospace)',
    fontSize: '13px',
    lineHeight: '1.65',
    overflow: 'auto',
  },
  '.cm-content': { padding: '8px 0' },
  '.cm-lineNumbers .cm-gutterElement': { minWidth: '3.5ch', textAlign: 'right' },
  '.cm-foldGutter': { display: 'none' },
  '.cm-tooltip': { borderRadius: '0' },
})

// ─── Component ─────────────────────────────────────────────────────────────────

type SqlEditorProps = {
  tabId: string
  /** The Y.Doc backing this editor. Must have a Y.Text at key 'content'. */
  doc: Y.Doc
  className?: string
  onCursorChange?: (line: number, col: number, selSize: number) => void
}

export function SqlEditor({ tabId, doc, className, onCursorChange }: SqlEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const viewRegistry = useEditorViewRegistry()
  const onCursorChangeRef = useRef(onCursorChange)
  onCursorChangeRef.current = onCursorChange

  const { resolvedTheme } = useTheme()
  const { editorThemeDark, editorThemeLight } = useEditorTheme()
  const activeThemeName = resolvedTheme === 'dark' ? editorThemeDark : editorThemeLight

  const themeCompartment = useRef(new Compartment())
  const viewRef = useRef<EditorView | null>(null)

  // Re-mount the editor whenever the active doc changes.
  // key={activeTab.id} at the call site also ensures clean remount on tab switch.
  useEffect(() => {
    if (!containerRef.current) return
    const yText = doc.getText('content')

    const view = new EditorView({
      state: EditorState.create({
        doc: yText.toString(),
        extensions: [
          basicSetup,
          sql(),
          baseEditorTheme,
          themeCompartment.current.of(getCachedTheme(activeThemeName) ?? []),
          EditorView.lineWrapping,
          yCollab(yText, null), // handles all CodeMirror ↔ Y.js sync
          EditorView.updateListener.of((update) => {
            if (!update.selectionSet && !update.docChanged) return
            const cb = onCursorChangeRef.current
            if (!cb) return
            const head = update.state.selection.main.head
            const line = update.state.doc.lineAt(head)
            cb(line.number, head - line.from + 1, update.state.selection.main.to - update.state.selection.main.from)
          }),
        ],
      }),
      parent: containerRef.current,
    })

    viewRef.current = view
    viewRegistry.register(tabId, view)

    return () => {
      viewRef.current = null
      viewRegistry.unregister(tabId)
      view.destroy()
    }
  }, [doc, tabId, viewRegistry])

  // Hot-swap the theme without remounting the editor.
  useEffect(() => {
    let cancelled = false
    loadEditorTheme(activeThemeName).then((ext) => {
      if (cancelled || !viewRef.current) return
      viewRef.current.dispatch({
        effects: themeCompartment.current.reconfigure(ext),
      })
    })
    return () => {
      cancelled = true
    }
  }, [activeThemeName])

  return <div ref={containerRef} className={cn('h-full overflow-hidden', className)} />
}
