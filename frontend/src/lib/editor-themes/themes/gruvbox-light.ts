import { EditorView } from '@codemirror/view'
import { syntaxHighlighting, HighlightStyle } from '@codemirror/language'
import { tags } from '@lezer/highlight'
import type { Extension } from '@codemirror/state'

const gruvboxLightTheme = EditorView.theme(
  {
    '&': { height: '100%', backgroundColor: '#fbf1c7', color: '#3c3836' },
    '.cm-scroller': {
      fontFamily: 'var(--font-mono, ui-monospace, monospace)',
      fontSize: '13px',
      lineHeight: '1.65',
      overflow: 'auto',
    },
    '.cm-content': { caretColor: '#3c3836', padding: '8px 0' },
    '.cm-gutters': {
      backgroundColor: '#f2e5bc',
      border: 'none',
      borderRight: '1px solid #d5c4a1',
      color: '#928374',
      paddingRight: '8px',
      userSelect: 'none',
    },
    '.cm-lineNumbers .cm-gutterElement': { minWidth: '3.5ch', textAlign: 'right' },
    '.cm-activeLine': { backgroundColor: 'rgba(213, 196, 161, 0.4)' },
    '.cm-activeLineGutter': { backgroundColor: 'transparent', color: '#3c3836' },
    '.cm-cursor, .cm-dropCursor': { borderLeftColor: '#3c3836' },
    '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection': {
      backgroundColor: 'rgba(213, 196, 161, 0.6) !important',
    },
    '.cm-matchingBracket': { backgroundColor: 'rgba(184, 187, 38, 0.25)', outline: 'none' },
    '.cm-tooltip': {
      backgroundColor: '#f2e5bc',
      border: '1px solid #d5c4a1',
      borderRadius: '0',
      boxShadow: '0 4px 12px rgb(0 0 0 / 0.1)',
    },
    '.cm-tooltip-autocomplete > ul > li[aria-selected]': {
      backgroundColor: '#d5c4a1',
      color: '#3c3836',
    },
    '.cm-foldGutter': { display: 'none' },
  },
  { dark: false },
)

const gruvboxLightHighlight = HighlightStyle.define([
  { tag: tags.keyword, color: '#9d0006', fontWeight: '600' },
  { tag: [tags.string, tags.special(tags.string)], color: '#79740e' },
  { tag: [tags.number, tags.bool], color: '#8f3f71' },
  { tag: tags.comment, color: '#928374', fontStyle: 'italic' },
  { tag: [tags.operator, tags.punctuation], color: '#3c3836' },
  { tag: tags.null, color: '#8f3f71', fontStyle: 'italic' },
  { tag: tags.variableName, color: '#3c3836' },
  { tag: tags.typeName, color: '#076678' },
])

const gruvboxLight: Extension = [gruvboxLightTheme, syntaxHighlighting(gruvboxLightHighlight)]

export default gruvboxLight
