import type { ReactNode } from 'react'
import { highlightCode, tagHighlighter, tags as t } from '@lezer/highlight'
import { sql } from '@codemirror/lang-sql'

const sqlLanguage = sql().language

// Fixed accent palette rather than the live editor theme, since this runs
// without mounting a CodeMirror EditorView.
const highlighter = tagHighlighter([
  { tag: t.keyword, class: 'font-semibold text-primary' },
  { tag: [t.string, t.special(t.string)], class: 'text-emerald-600 dark:text-emerald-400' },
  { tag: [t.number, t.bool], class: 'text-amber-600 dark:text-amber-400' },
  { tag: t.comment, class: 'italic text-muted-foreground' },
  { tag: t.typeName, class: 'text-sky-600 dark:text-sky-400' },
  { tag: t.null, class: 'italic text-sky-600 dark:text-sky-400' },
])

/** Statically syntax-highlights a SQL string for compact previews (e.g. a
 *  collapsed history row) without mounting a CodeMirror editor per row. */
export function highlightSqlStatic(value: string): ReactNode {
  const tree = sqlLanguage.parser.parse(value)
  const nodes: ReactNode[] = []
  let key = 0
  highlightCode(
    value,
    tree,
    highlighter,
    (text, classes) => {
      nodes.push(
        classes ? (
          <span key={key++} className={classes}>
            {text}
          </span>
        ) : (
          text
        ),
      )
    },
    () => {
      nodes.push('\n')
    },
  )
  return nodes
}
