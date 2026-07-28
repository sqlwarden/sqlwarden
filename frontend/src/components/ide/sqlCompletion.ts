import {
  acceptCompletion,
  autocompletion,
  type Completion,
  type CompletionContext,
  type CompletionResult,
  type CompletionSource,
} from '@codemirror/autocomplete'
import {
  MySQL,
  PostgreSQL,
  SQLite,
  StandardSQL,
  keywordCompletionSource,
  sql,
  type SQLDialect,
} from '@codemirror/lang-sql'
import type { Extension } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { buildIcon, getIcon } from '@iconify/react'
import {
  completeConnectionSQL,
  getSQLCompletionVocabulary,
  type SQLCompletionSuggestion,
} from '#/lib/api/queries/database'
import type { AppIcon } from '#/lib/icons'

type IconMap = Partial<Record<AppIcon, string>>

export type SQLCompletionConfig = {
  orgSlug?: string
  workspaceId?: number
  connectionId?: number
  driver?: string
  sessionId?: string
  iconMap?: IconMap | null
}

export function dialectForDriver(driver?: string): SQLDialect {
  switch (driver?.toLowerCase()) {
    case 'postgres':
    case 'postgresql':
      return PostgreSQL
    case 'mysql':
    case 'mariadb':
      return MySQL
    case 'sqlite':
    case 'sqlite3':
      return SQLite
    default:
      return StandardSQL
  }
}

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

function renderCompletionIcon(completion: Completion, iconMap?: IconMap | null): Node {
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

const completionTheme = EditorView.theme({
  '.cm-tooltip.cm-tooltip-autocomplete': {
    backgroundColor: 'var(--popover)',
    color: 'var(--popover-foreground)',
    border: '1px solid var(--border)',
    borderRadius: 'var(--radius)',
    boxShadow:
      '0 12px 32px color-mix(in oklab, var(--foreground) 12%, transparent), 0 2px 8px color-mix(in oklab, var(--foreground) 8%, transparent)',
    overflow: 'hidden',
    fontFamily: 'var(--font-interface)',
  },
  '.cm-tooltip-autocomplete > ul': {
    minWidth: '18rem',
    maxWidth: '32rem',
    maxHeight: 'min(22rem, 45vh)',
    padding: '0.25rem',
  },
  '.cm-tooltip-autocomplete > ul > li': {
    display: 'flex',
    alignItems: 'center',
    gap: '0.5rem',
    minHeight: '1.875rem',
    padding: '0.375rem 0.5rem',
    borderRadius: 'calc(var(--radius) * 0.6)',
    lineHeight: '1.25',
  },
  '.cm-tooltip-autocomplete > ul > li[aria-selected]': {
    backgroundColor: 'var(--accent)',
    color: 'var(--accent-foreground)',
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
  '.cm-completionLabel': {
    minWidth: '0',
    flex: '1',
    fontFamily: 'var(--font-data)',
    fontSize: '0.75rem',
  },
  '.cm-completionMatchedText': {
    color: 'var(--primary)',
    fontWeight: '650',
    textDecoration: 'none',
  },
  '.cm-completionDetail': {
    maxWidth: '14rem',
    marginLeft: 'auto',
    overflow: 'hidden',
    color: 'var(--muted-foreground)',
    fontFamily: 'var(--font-interface)',
    fontSize: '0.6875rem',
    fontStyle: 'normal',
    textOverflow: 'ellipsis',
    whiteSpace: 'nowrap',
  },
  '.cm-tooltip-autocomplete > ul > li[aria-selected] .cm-completionDetail': {
    color: 'var(--accent-foreground)',
    opacity: '0.7',
  },
})

// CodeMirror deliberately leaves Tab unbound so it can move focus out of the
// editor. Consume it only while a highlighted completion can be accepted.
export function acceptCompletionOnTab(view: EditorView, event: KeyboardEvent): boolean {
  if (event.key !== 'Tab' || event.shiftKey || event.metaKey || event.ctrlKey || event.altKey) {
    return false
  }
  return acceptCompletion(view)
}

const completionTabHandler = EditorView.domEventHandlers({
  keydown(event, view) {
    return acceptCompletionOnTab(view, event)
  },
})

const vocabularyCache = new Map<string, Promise<SQLCompletionSuggestion[]>>()
const STRUCTURAL_TRIGGERS = new Set(['.', ' ', ',', '('])
const IDENTIFIER_VALID_FOR = /^[\w$]*$/

function normalizedDriver(driver?: string): string {
  switch (driver?.toLowerCase()) {
    case 'postgresql':
      return 'postgres'
    case 'mariadb':
      return 'mysql'
    case 'sqlite3':
      return 'sqlite'
    default:
      return driver?.toLowerCase() || 'standard'
  }
}

function loadVocabulary(driver: string): Promise<SQLCompletionSuggestion[]> {
  const key = normalizedDriver(driver)
  const cached = vocabularyCache.get(key)
  if (cached) return cached
  const pending = getSQLCompletionVocabulary(key)
    .then((result) => result.suggestions)
    // Keep the failed lookup memoized as an empty vocabulary for this page.
    // Otherwise an unavailable endpoint would be retried on every identifier.
    .catch(() => [])
  vocabularyCache.set(key, pending)
  return pending
}

export function clearSQLCompletionVocabularyCache(): void {
  vocabularyCache.clear()
}

function suggestionToCompletion(suggestion: SQLCompletionSuggestion): Completion {
  return {
    label: suggestion.label,
    displayLabel: suggestion.display_label,
    type: suggestion.kind,
    detail: suggestion.detail,
    apply: suggestion.insert_text || suggestion.label,
    boost: suggestion.score,
  }
}

function mergeCompletions(primary: Completion[], secondary: Completion[]): Completion[] {
  const result: Completion[] = []
  const seen = new Set<string>()
  for (const completion of [...primary, ...secondary]) {
    const key = `${completion.type || ''}\0${completion.label.toLocaleLowerCase()}`
    if (seen.has(key)) continue
    seen.add(key)
    result.push(completion)
  }
  return result
}

export function remoteSQLCompletionSource(config: SQLCompletionConfig): CompletionSource {
  const dialect = dialectForDriver(config.driver)
  const localKeywords = keywordCompletionSource(dialect, true)
  let activeRemoteController: AbortController | undefined
  let remoteGeneration = 0

  return async (context: CompletionContext): Promise<CompletionResult | null> => {
    const word = context.matchBefore(/[A-Za-z_$][\w$]*$/)
    const prefix = word?.text ?? ''
    const previous = context.pos > 0 ? context.state.sliceDoc(context.pos - 1, context.pos) : ''
    const structural = STRUCTURAL_TRIGGERS.has(previous)
    const shouldCompleteLexically = context.explicit || prefix.length >= 2
    const hasRemote =
      config.orgSlug !== undefined &&
      config.workspaceId !== undefined &&
      config.connectionId !== undefined &&
      config.driver !== undefined
    const shouldCompleteRemotely = hasRemote && (context.explicit || structural)

    let lexical: Completion[] = []
    if (shouldCompleteLexically && config.driver) {
      try {
        const vocabulary = await loadVocabulary(config.driver)
        if (vocabulary.length === 0) {
          const fallback = await localKeywords(context)
          lexical = [...(fallback?.options ?? [])]
        } else {
          const foldedPrefix = prefix.toLocaleLowerCase()
          lexical = vocabulary
            .filter(
              (suggestion) =>
                context.explicit || suggestion.label.toLocaleLowerCase().startsWith(foldedPrefix),
            )
            .map(suggestionToCompletion)
        }
      } catch (_error) {
        const fallback = await localKeywords(context)
        lexical = [...(fallback?.options ?? [])]
      }
    }

    if (!shouldCompleteRemotely) {
      if (lexical.length === 0) return null
      return {
        from: word?.from ?? context.pos,
        to: context.pos,
        options: lexical,
        validFor: IDENTIFIER_VALID_FOR,
      }
    }

    activeRemoteController?.abort()
    const controller = new AbortController()
    activeRemoteController = controller
    const generation = ++remoteGeneration
    context.addEventListener('abort', () => controller.abort())
    try {
      const result = await completeConnectionSQL(
        config.orgSlug!,
        config.workspaceId!,
        config.connectionId!,
        context.state.doc.toString(),
        context.pos,
        config.sessionId,
        controller.signal,
        context.explicit ? 'invoked' : 'automatic',
        structural ? previous : undefined,
      )
      if (context.aborted || controller.signal.aborted || generation !== remoteGeneration)
        return null
      const from = result.suggestions[0]?.replace_start ?? word?.from ?? context.pos
      const semantic = result.suggestions
        .filter((suggestion) => suggestion.replace_start === from)
        .map(suggestionToCompletion)
      const options = mergeCompletions(semantic, lexical)
      if (options.length === 0) return null
      return {
        from: Math.max(0, Math.min(from, context.pos)),
        to: context.pos,
        options,
        validFor: IDENTIFIER_VALID_FOR,
      }
    } catch (_error) {
      if (controller.signal.aborted || context.aborted || generation !== remoteGeneration)
        return null
      if (lexical.length > 0) {
        return {
          from: word?.from ?? context.pos,
          to: context.pos,
          options: lexical,
          validFor: IDENTIFIER_VALID_FOR,
        }
      }
      return localKeywords(context)
    } finally {
      if (activeRemoteController === controller) {
        activeRemoteController = undefined
      }
    }
  }
}

export function sqlCompletionExtension(config: SQLCompletionConfig): Extension {
  const dialect = dialectForDriver(config.driver)
  const remote =
    config.driver !== 'sqlite' && config.driver !== 'sqlite3' && config.connectionId !== undefined
  return [
    sql({ dialect, upperCaseKeywords: true }),
    autocompletion({
      activateOnTypingDelay: 150,
      icons: false,
      addToOptions: [
        {
          position: 20,
          render: (completion) => renderCompletionIcon(completion, config.iconMap),
        },
      ],
      ...(remote ? { override: [remoteSQLCompletionSource(config)] } : {}),
    }),
    completionTheme,
    completionTabHandler,
  ]
}
