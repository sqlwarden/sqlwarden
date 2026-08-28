import { closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete'
import { defaultKeymap, history, historyKeymap } from '@codemirror/commands'
import {
  bracketMatching,
  defaultHighlightStyle,
  foldGutter,
  foldKeymap,
  indentOnInput,
  syntaxHighlighting,
} from '@codemirror/language'
import { lintKeymap } from '@codemirror/lint'
import { highlightSelectionMatches, search, searchKeymap } from '@codemirror/search'
import { EditorState, type Extension } from '@codemirror/state'
import {
  crosshairCursor,
  drawSelection,
  dropCursor,
  EditorView,
  highlightActiveLine,
  highlightActiveLineGutter,
  highlightSpecialChars,
  keymap,
  lineNumbers,
  rectangularSelection,
} from '@codemirror/view'
import { createFindPanel } from './findPanel'
import { IDENTIFIER_DND_MIME } from './sqlDialect'

// The find panel supplies its own design-system chrome, so strip CodeMirror's
// default panel border/background and theme the in-document match highlights
// with the app's tokens. var(...) values pass straight through to CSS.
const sqlwardenSearchTheme = EditorView.theme({
  '.cm-panels': { backgroundColor: 'transparent', color: 'inherit', border: 'none' },
  '.cm-panels.cm-panels-top': { borderBottom: '1px solid var(--border)' },
  '.cm-panel': { padding: '0', margin: '0' },
  '.cm-searchMatch': {
    backgroundColor: 'color-mix(in oklab, var(--ring) 25%, transparent)',
    borderRadius: '2px',
  },
  '.cm-searchMatch-selected': {
    backgroundColor: 'color-mix(in oklab, var(--ring) 55%, transparent)',
  },
  // highlightSelectionMatches() defaults to a hardcoded #99ff7780 green wash
  // that clashes with same-hue syntax tokens (e.g. string literals) and is
  // barely visible on a light background. A low-alpha neutral fill (no
  // border — VS Code's word-highlight is a plain filled box) keeps text
  // readable under every token color while staying visible against the
  // editor background. A plain theme() extension overrides the library's
  // baseTheme rule at equal specificity, same as .cm-searchMatch above.
  // .cm-selectionMatch-main (the exact selected occurrence) inherits the
  // same fill rather than being left transparent — otherwise the cursor's
  // own line reads as unhighlighted next to every other matching line.
  '.cm-selectionMatch': {
    backgroundColor: 'color-mix(in oklab, var(--color-foreground) 14%, transparent)',
    borderRadius: '2px',
  },
})

// Inserts a dragged schema identifier at the drop position. Returns false for any
// other drop (e.g. CodeMirror's own text drags) so default handling stays intact.
const schemaDropHandler = EditorView.domEventHandlers({
  drop(event, view) {
    const text = event.dataTransfer?.getData(IDENTIFIER_DND_MIME)
    if (!text) return false
    event.preventDefault()
    const pos =
      view.posAtCoords({ x: event.clientX, y: event.clientY }) ?? view.state.selection.main.head
    view.dispatch({
      changes: { from: pos, insert: text },
      selection: { anchor: pos + text.length },
    })
    view.focus()
    return true
  },
})

export const sqlwardenBasicSetup: Extension = [
  lineNumbers(),
  highlightActiveLineGutter(),
  highlightSpecialChars(),
  history(),
  foldGutter(),
  drawSelection(),
  dropCursor(),
  EditorState.allowMultipleSelections.of(true),
  indentOnInput(),
  syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
  bracketMatching(),
  closeBrackets(),
  rectangularSelection(),
  crosshairCursor(),
  highlightActiveLine(),
  highlightSelectionMatches(),
  search({ top: true, createPanel: createFindPanel }),
  sqlwardenSearchTheme,
  schemaDropHandler,
  keymap.of([
    ...closeBracketsKeymap,
    ...defaultKeymap,
    ...searchKeymap,
    ...historyKeymap,
    ...foldKeymap,
    ...lintKeymap,
  ]),
]
