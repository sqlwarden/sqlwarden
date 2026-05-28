import { useEffect, useRef } from 'react'
import { EditorView, basicSetup } from 'codemirror'
import { EditorState } from '@codemirror/state'
import { sql } from '@codemirror/lang-sql'
import { syntaxHighlighting, HighlightStyle } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import { yCollab } from 'y-codemirror.next'
import type * as Y from 'yjs'
import { cn } from '#/lib/utils'

// ─── Theme ─────────────────────────────────────────────────────────────────────

const ideTheme = EditorView.theme(
  {
    '&': { height: '100%', backgroundColor: 'transparent' },
    '.cm-scroller': {
      fontFamily: 'var(--font-mono, ui-monospace, monospace)',
      fontSize: '13px',
      lineHeight: '1.65',
      overflow: 'auto',
    },
    '.cm-content': { caretColor: 'var(--color-foreground)', padding: '8px 0' },
    '.cm-gutters': {
      backgroundColor: 'transparent',
      border: 'none',
      borderRight: '1px solid color-mix(in oklch, var(--color-border) 80%, transparent)',
      color: 'var(--color-muted-foreground)',
      paddingRight: '8px',
      userSelect: 'none',
    },
    '.cm-lineNumbers .cm-gutterElement': { minWidth: '3.5ch', textAlign: 'right' },
    '.cm-activeLine': {
      backgroundColor: 'color-mix(in oklch, var(--color-muted) 40%, transparent)',
    },
    '.cm-activeLineGutter': {
      backgroundColor: 'transparent',
      color: 'var(--color-foreground)',
    },
    '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--color-foreground)' },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground': {
      backgroundColor: 'color-mix(in oklch, var(--color-primary) 22%, transparent)',
    },
    '.cm-matchingBracket': {
      backgroundColor: 'color-mix(in oklch, var(--color-primary) 15%, transparent)',
      outline: 'none',
    },
    '.cm-tooltip': {
      backgroundColor: 'var(--color-popover)',
      border: '1px solid var(--color-border)',
      borderRadius: '0',
      boxShadow: '0 4px 12px rgb(0 0 0 / 0.15)',
    },
    '.cm-tooltip-autocomplete > ul > li[aria-selected]': {
      backgroundColor: 'var(--color-accent)',
      color: 'var(--color-accent-foreground)',
    },
    '.cm-foldGutter': { display: 'none' },
  },
  { dark: true },
)

// ─── Syntax highlighting ────────────────────────────────────────────────────────

const sqlHighlightStyle = HighlightStyle.define([
  { tag: tags.keyword, color: 'var(--color-primary)', fontWeight: '600' },
  { tag: [tags.string, tags.special(tags.string)], color: 'var(--color-chart-1)' },
  { tag: [tags.number, tags.bool], color: 'var(--color-chart-2)' },
  { tag: tags.comment, color: 'var(--color-muted-foreground)', fontStyle: 'italic' },
  { tag: [tags.operator, tags.punctuation], color: 'var(--color-foreground)' },
  { tag: tags.null, color: 'var(--color-muted-foreground)', fontStyle: 'italic' },
  { tag: tags.variableName, color: 'var(--color-foreground)' },
  { tag: tags.typeName, color: 'var(--color-chart-3)' },
])

// ─── Component ─────────────────────────────────────────────────────────────────

type SqlEditorProps = {
  /** The Y.Doc backing this editor. Must have a Y.Text at key 'content'. */
  doc: Y.Doc
  className?: string
}

export function SqlEditor({ doc, className }: SqlEditorProps) {
  const containerRef = useRef<HTMLDivElement>(null)

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
          ideTheme,
          syntaxHighlighting(sqlHighlightStyle),
          EditorView.lineWrapping,
          yCollab(yText), // handles all CodeMirror ↔ Y.js sync
        ],
      }),
      parent: containerRef.current,
    })

    return () => view.destroy()
  }, [doc])

  return <div ref={containerRef} className={cn('h-full overflow-hidden', className)} />
}
