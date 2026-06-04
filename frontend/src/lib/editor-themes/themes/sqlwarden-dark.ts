import { EditorView } from '@codemirror/view'
import { syntaxHighlighting, HighlightStyle } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import type { Extension } from '@codemirror/state'

const ideThemeDark = EditorView.theme(
  {
    '&': { height: '100%', backgroundColor: 'transparent', color: 'oklch(from var(--color-foreground) 0.97 c h)' },
    '.cm-content': { caretColor: 'oklch(from var(--color-foreground) 0.97 c h)', padding: '8px 0' },
    '.cm-gutters': {
      backgroundColor: 'transparent',
      border: 'none',
      borderRight: '1px solid color-mix(in oklch, var(--color-border) 80%, transparent)',
      color: 'var(--color-muted-foreground)',
      paddingRight: '8px',
      userSelect: 'none',
    },
    '.cm-activeLine': {
      backgroundColor: 'color-mix(in oklch, var(--color-muted) 40%, transparent)',
    },
    '.cm-activeLineGutter': {
      backgroundColor: 'transparent',
      color: 'var(--color-foreground)',
    },
    '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--color-foreground)' },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection': {
      backgroundColor: 'oklch(from var(--color-primary) 0.6 0.14 h / 38%) !important',
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
      color: 'var(--color-popover-foreground)',
    },
    '.cm-tooltip-autocomplete > ul > li[aria-selected]': {
      backgroundColor: 'var(--color-accent)',
      color: 'var(--color-accent-foreground)',
    },
  },
  { dark: true },
)

// All syntax colors derived from the primary hue via CSS relative color syntax.
// Hue rotations give semantically distinct colors while staying visually linked
// to the app's primary: +120° → strings, +240° → numbers, +185° → type names.
// L=0.78 gives brightness readable on a dark background.
const sqlHighlightStyleDark = HighlightStyle.define([
  { tag: tags.keyword,                              color: 'var(--color-primary)',                                          fontWeight: '600' },
  { tag: [tags.string, tags.special(tags.string)], color: 'oklch(from var(--color-primary) 0.78 0.14 calc(h + 120))' },
  { tag: [tags.number, tags.bool],                 color: 'oklch(from var(--color-primary) 0.78 0.12 calc(h + 240))' },
  { tag: tags.comment,                             color: 'var(--color-muted-foreground)',                                 fontStyle: 'italic' },
  { tag: [tags.operator, tags.punctuation],        color: 'oklch(from var(--color-foreground) 0.97 c h)' },
  { tag: tags.null,                                color: 'oklch(from var(--color-primary) 0.68 0.10 calc(h + 240))',      fontStyle: 'italic' },
  { tag: tags.variableName,                        color: 'oklch(from var(--color-foreground) 0.97 c h)' },
  { tag: tags.typeName,                            color: 'oklch(from var(--color-primary) 0.72 0.12 calc(h + 185))' },
])

const sqlwardenDark: Extension = [ideThemeDark, syntaxHighlighting(sqlHighlightStyleDark)]

export default sqlwardenDark
