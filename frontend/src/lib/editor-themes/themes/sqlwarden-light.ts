import { EditorView } from '@codemirror/view'
import { syntaxHighlighting, HighlightStyle } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import type { Extension } from '@codemirror/state'

const ideThemeLight = EditorView.theme(
  {
    '&': { height: '100%', backgroundColor: 'transparent' },
    '.cm-content': { caretColor: 'var(--color-foreground)', padding: '8px 0' },
    '.cm-gutters': {
      backgroundColor: 'transparent',
      border: 'none',
      borderRight: '1px solid color-mix(in oklch, var(--color-border) 80%, transparent)',
      color: 'var(--color-muted-foreground)',
      paddingRight: '8px',
      userSelect: 'none',
    },
    '.cm-activeLine': {
      backgroundColor: 'color-mix(in oklch, var(--color-muted) 50%, transparent)',
    },
    '.cm-activeLineGutter': {
      backgroundColor: 'transparent',
      color: 'var(--color-foreground)',
    },
    '.cm-cursor, .cm-dropCursor': { borderLeftColor: 'var(--color-foreground)' },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection': {
      backgroundColor: 'oklch(from var(--color-primary) 0.6 0.14 h / 25%) !important',
    },
    '.cm-matchingBracket': {
      backgroundColor: 'color-mix(in oklch, var(--color-primary) 20%, transparent)',
      outline: 'none',
    },
    '.cm-tooltip': {
      backgroundColor: 'var(--color-popover)',
      border: '1px solid var(--color-border)',
      borderRadius: '0',
      boxShadow: '0 4px 12px rgb(0 0 0 / 0.1)',
      color: 'var(--color-popover-foreground)',
    },
    '.cm-tooltip-autocomplete > ul > li[aria-selected]': {
      backgroundColor: 'var(--color-accent)',
      color: 'var(--color-accent-foreground)',
    },
  },
  { dark: false },
)

// Same hue-rotation strategy as sqlwarden-dark but with L=0.42 for readability
// on a light background. Comments use var(--color-muted-foreground) which
// already adapts to the mode.
const sqlHighlightStyleLight = HighlightStyle.define([
  { tag: tags.keyword,                              color: 'var(--color-primary)',                                          fontWeight: '600' },
  { tag: [tags.string, tags.special(tags.string)], color: 'oklch(from var(--color-primary) 0.42 0.14 calc(h + 120))' },
  { tag: [tags.number, tags.bool],                 color: 'oklch(from var(--color-primary) 0.42 0.12 calc(h + 240))' },
  { tag: tags.comment,                             color: 'var(--color-muted-foreground)',                                 fontStyle: 'italic' },
  { tag: [tags.operator, tags.punctuation],        color: 'var(--color-foreground)' },
  { tag: tags.null,                                color: 'oklch(from var(--color-primary) 0.45 0.10 calc(h + 240))',      fontStyle: 'italic' },
  { tag: tags.variableName,                        color: 'var(--color-foreground)' },
  { tag: tags.typeName,                            color: 'oklch(from var(--color-primary) 0.40 0.12 calc(h + 185))' },
])

const sqlwardenLight: Extension = [ideThemeLight, syntaxHighlighting(sqlHighlightStyleLight)]

export default sqlwardenLight
