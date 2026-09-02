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
import { RangeSetBuilder, EditorState, type Extension } from '@codemirror/state'
import {
  crosshairCursor,
  Decoration,
  drawSelection,
  dropCursor,
  EditorView,
  highlightActiveLine,
  highlightActiveLineGutter,
  highlightSpecialChars,
  keymap,
  lineNumbers,
  rectangularSelection,
  ViewPlugin,
  type DecorationSet,
  type ViewUpdate,
} from '@codemirror/view'
import { indentationMarkers } from '@replit/codemirror-indentation-markers'
import { showMinimap } from '@replit/codemirror-minimap'
import { createFindPanel } from './codemirrorFindPanel'
import { IDENTIFIER_DND_MIME } from './sqlDialect'

// A common convention for formatted SQL line width; matches no specific
// formatter setting today, just gives a visual reference while writing.
const RULER_COLUMN = 100

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
  // Column ruler: a single-pixel vertical guide at RULER_COLUMN, drawn as a
  // background image on .cm-content rather than a decoration — it needs no
  // per-line computation and scales with document height automatically.
  '.cm-content': {
    backgroundImage: 'linear-gradient(var(--color-border) 100%, transparent 100%)',
    backgroundRepeat: 'no-repeat',
    backgroundSize: '1px 100%',
    backgroundPosition: `${RULER_COLUMN}ch 0`,
  },
  // Neutral, low-alpha tint rather than a warning color — trailing whitespace
  // is a style nit, not an error, and VS Code's own rendering is similarly muted.
  '.cm-trailingWhitespace': {
    backgroundColor: 'color-mix(in oklab, var(--color-muted-foreground) 25%, transparent)',
    borderRadius: '2px',
  },
})

const trailingWhitespaceMark = Decoration.mark({ class: 'cm-trailingWhitespace' })

function findTrailingWhitespace(view: EditorView): DecorationSet {
  const builder = new RangeSetBuilder<Decoration>()
  for (const { from, to } of view.visibleRanges) {
    let pos = from
    while (pos <= to) {
      const line = view.state.doc.lineAt(pos)
      const match = /[ \t]+$/.exec(line.text)
      if (match) {
        const start = line.from + match.index
        if (start < line.to) builder.add(start, line.to, trailingWhitespaceMark)
      }
      pos = line.to + 1
    }
  }
  return builder.finish()
}

// Flags trailing whitespace so it's visible instead of silently riding along
// in pasted or hand-edited SQL, where it can confuse formatters and diffs.
const highlightTrailingWhitespace = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet
    constructor(view: EditorView) {
      this.decorations = findTrailingWhitespace(view)
    }
    update(update: ViewUpdate) {
      if (update.docChanged || update.viewportChanged) {
        this.decorations = findTrailingWhitespace(update.view)
      }
    }
  },
  { decorations: (plugin) => plugin.decorations },
)

// Renders text as scaled-down colored blocks (VS Code's default) rather than
// tiny glyphs, which stays legible at minimap scale; canvas fill colors are
// read from each token's computed style, so it tracks the active syntax
// theme automatically without any color config here.
const editorMinimap = showMinimap.of({
  create: () => ({ dom: document.createElement('div') }),
  displayText: 'blocks',
  showOverlay: 'mouse-over',
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
  highlightTrailingWhitespace,
  // Active-block guide is a stronger shade of the same neutral, not a brand
  // accent — VS Code's indent guides stay grayscale even when highlighted.
  indentationMarkers({
    highlightActiveBlock: true,
    hideFirstIndent: true,
    colors: {
      light: 'var(--color-border)',
      dark: 'var(--color-border)',
      activeLight: 'color-mix(in oklab, var(--color-foreground) 35%, transparent)',
      activeDark: 'color-mix(in oklab, var(--color-foreground) 35%, transparent)',
    },
  }),
  editorMinimap,
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
