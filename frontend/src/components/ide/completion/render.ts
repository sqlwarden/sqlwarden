import { type Completion } from '@codemirror/autocomplete'
import { EditorView } from '@codemirror/view'
import { buildIcon, getIcon } from '@iconify/react'
import type { AppIcon } from '#/lib/icons'
import type { SQLCompletionSuggestion } from '#/lib/api/queries/database'

export type IconMap = Partial<Record<AppIcon, string>>

const COMPLETION_ICONS: Record<string, AppIcon> = {
  column: 'column',
  schema: 'database',
  database: 'database',
  table: 'table',
  view: 'eye',
  materialized_view: 'eye',
  function: 'play',
  procedure: 'terminal',
  sequence: 'sort',
  trigger: 'flow-connection',
  index: 'sort',
  event: 'flow-connection',
  engine: 'server-stack-01',
  keyword: 'key-01',
  type: 'subject',
  charset: 'subject',
}

export function renderCompletionIcon(completion: Completion, iconMap?: IconMap | null): Node {
  const kind = completion.type || 'text'
  const container = document.createElement('span')
  container.className = `cm-completionKindIcon cm-completionKindIcon-${kind}`
  container.dataset.kind = kind
  container.setAttribute('aria-hidden', 'true')

  const appIcon = COMPLETION_ICONS[kind] ?? 'subject'
  const iconName = iconMap?.[appIcon]
  const icon = iconName ? getIcon(iconName) : null
  if (!icon) {
    container.textContent = kind.slice(0, 1).toUpperCase()
    return container
  }

  const rendered = buildIcon(icon, { height: '1em', width: '1em' })
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg')
  for (const [name, value] of Object.entries(rendered.attributes)) {
    if (value !== undefined) svg.setAttribute(name, value)
  }
  svg.setAttribute('fill', 'currentColor')
  svg.setAttribute('focusable', 'false')
  svg.innerHTML = rendered.body
  container.append(svg)
  return container
}

const LEGACY_DETAIL = /^(?:(\w+)\.)?(\w+)\s*\|\s*([^,]+)/

function legacyColumnContext(detail: string | undefined): string | undefined {
  const match = detail?.match(LEGACY_DETAIL)
  if (!match) return undefined
  return `${match[2]} · ${match[3].trim()}`
}

function schemaFromDetail(detail: string | undefined): string | undefined {
  return detail?.match(/^(\w+)\./)?.[1]
}

// Short, right-aligned descriptor shown after the label. Prefers the
// structured fields the backend now sends and only parses the legacy
// "schema.table | type" detail string as a fallback for older payloads.
export function suggestionContextText(suggestion: SQLCompletionSuggestion): string {
  switch (suggestion.kind) {
    case 'column':
      if (suggestion.qualifier) {
        return suggestion.data_type
          ? `${suggestion.qualifier} · ${suggestion.data_type}`
          : suggestion.qualifier
      }
      return legacyColumnContext(suggestion.detail) ?? 'column'
    case 'table':
    case 'view':
    case 'materialized_view':
    case 'foreign_table':
      return suggestion.namespace || schemaFromDetail(suggestion.detail) || ''
    case 'function':
    case 'procedure':
      return 'function'
    case 'type':
      return 'type'
    case 'keyword':
      return 'keyword'
    case 'schema':
    case 'database':
      return 'schema'
    default:
      return suggestion.detail ?? ''
  }
}

function renderCompletionLabel(completion: Completion, match?: readonly number[]): HTMLSpanElement {
  const label = document.createElement('span')
  label.className = 'cm-completionLabel'
  const text = completion.displayLabel ?? completion.label
  if (!match || match.length === 0) {
    label.textContent = text
    return label
  }
  let offset = 0
  for (let index = 0; index < match.length;) {
    const from = match[index++]
    const to = match[index++]
    if (from > offset) label.append(document.createTextNode(text.slice(offset, from)))
    const matched = label.appendChild(document.createElement('span'))
    matched.className = 'cm-completionMatchedText'
    matched.textContent = text.slice(from, to)
    offset = to
  }
  if (offset < text.length) label.append(document.createTextNode(text.slice(offset)))
  return label
}

// Single-line row: [icon] [label] [flex spacer] [context]. Replaces
// CodeMirror's default label/detail slots (hidden via completionTheme) so
// truncation and alignment are controlled here.
export function renderCompletionRow(
  completion: Completion,
  iconMap?: IconMap | null,
  match?: readonly number[],
): HTMLElement {
  const row = document.createElement('div')
  row.className = 'cm-completionRow'
  row.append(renderCompletionIcon(completion, iconMap))
  row.append(renderCompletionLabel(completion, match))

  const context = document.createElement('span')
  context.className = 'cm-completionContext'
  context.textContent = completion.detail ?? ''
  row.append(context)
  return row
}

export const completionTheme = EditorView.theme({
  '.cm-tooltip.cm-tooltip-autocomplete': {
    backgroundColor: 'var(--popover)',
    color: 'var(--popover-foreground)',
    border: '1px solid var(--border)',
    borderRadius: 'var(--radius)',
    overflow: 'hidden',
    fontFamily: 'var(--font-interface)',
  },
  '.cm-tooltip-autocomplete > ul': {
    minWidth: '16rem',
    maxWidth: '30rem',
    maxHeight: 'min(22rem, 45vh)',
    padding: '0.25rem',
  },
  '.cm-tooltip-autocomplete > ul > li': {
    display: 'flex',
    alignItems: 'center',
    minHeight: '1.5rem',
    padding: '0.25rem 0.5rem',
    borderRadius: 'calc(var(--radius) * 0.6)',
    lineHeight: '1.25',
  },
  '.cm-tooltip-autocomplete > ul > li[aria-selected]': {
    backgroundColor: 'var(--accent)',
    color: 'var(--accent-foreground)',
  },
  // Hide CodeMirror's default label/detail slots; renderCompletionRow
  // draws the full row as a single direct child of the <li>.
  '.cm-tooltip-autocomplete > ul > li > .cm-completionLabel, .cm-tooltip-autocomplete > ul > li > .cm-completionDetail':
    {
      display: 'none',
    },
  '.cm-completionRow': {
    display: 'flex',
    alignItems: 'center',
    gap: '0.5rem',
    width: '100%',
    minWidth: '0',
  },
  '.cm-completionKindIcon': {
    display: 'inline-flex',
    width: '1rem',
    height: '1rem',
    flexShrink: '0',
    alignItems: 'center',
    justifyContent: 'center',
    color: 'var(--muted-foreground)',
    fontFamily: 'var(--font-interface)',
    fontSize: '0.625rem',
    fontWeight: '600',
  },
  '.cm-completionKindIcon svg': {
    width: '1rem',
    height: '1rem',
  },
  '.cm-completionKindIcon-column': { color: 'var(--primary)' },
  '.cm-completionKindIcon-table': { color: 'var(--chart-4)' },
  '.cm-completionKindIcon-view, .cm-completionKindIcon-materialized_view': {
    color: 'var(--chart-2)',
  },
  '.cm-completionKindIcon-function, .cm-completionKindIcon-procedure': {
    color: 'var(--chart-1)',
  },
  '.cm-completionKindIcon-sequence, .cm-completionKindIcon-index': {
    color: 'var(--chart-3)',
  },
  '.cm-completionKindIcon-trigger, .cm-completionKindIcon-event': {
    color: 'var(--chart-5)',
  },
  '.cm-completionRow .cm-completionLabel': {
    display: 'block',
    minWidth: '0',
    flex: '1',
    overflow: 'hidden',
    fontFamily: 'var(--font-data)',
    fontSize: '0.75rem',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  '.cm-completionMatchedText': {
    color: 'var(--primary)',
    fontWeight: '650',
    textDecoration: 'none',
  },
  '.cm-completionContext': {
    maxWidth: '12rem',
    marginLeft: 'auto',
    paddingLeft: '0.75rem',
    overflow: 'hidden',
    color: 'var(--muted-foreground)',
    fontFamily: 'var(--font-interface)',
    fontSize: '0.6875rem',
    fontStyle: 'normal',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  '.cm-tooltip-autocomplete > ul > li[aria-selected] .cm-completionContext': {
    color: 'var(--accent-foreground)',
    opacity: '0.7',
  },
})
