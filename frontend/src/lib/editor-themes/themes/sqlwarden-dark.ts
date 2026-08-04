import { EditorView } from '@codemirror/view'
import { syntaxHighlighting, HighlightStyle } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import type { Extension } from '@codemirror/state'

const ideThemeDark = EditorView.theme(
  {
    '&': {
      height: '100%',
      backgroundColor: 'transparent',
      color: 'oklch(from var(--color-foreground) 0.97 c h)',
    },
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

// Keyword stays tied to the brand primary; every other token gets its own
// fixed, genuinely distinct hue (Nord-inspired: soft green/amber/lavender/teal)
// rather than a rotation of the primary, which read as monochromatic blue.
const sqlHighlightStyleDark = HighlightStyle.define([
  { tag: tags.keyword, color: 'var(--color-primary)', fontWeight: '600' },
  { tag: [tags.string, tags.special(tags.string)], color: 'oklch(0.75 0.13 145)' },
  { tag: [tags.number, tags.bool], color: 'oklch(0.78 0.13 55)' },
  { tag: tags.comment, color: 'var(--color-muted-foreground)', fontStyle: 'italic' },
  { tag: [tags.operator, tags.punctuation], color: 'oklch(from var(--color-foreground) 0.97 c h)' },
  { tag: tags.null, color: 'oklch(0.72 0.11 300)', fontStyle: 'italic' },
  { tag: tags.variableName, color: 'oklch(from var(--color-foreground) 0.97 c h)' },
  { tag: tags.typeName, color: 'oklch(0.78 0.10 195)' },
])

const sqlwardenDark: Extension = [ideThemeDark, syntaxHighlighting(sqlHighlightStyleDark)]

export default sqlwardenDark
