import {
  acceptCompletion,
  autocompletion,
  closeCompletion,
  completionStatus,
  moveCompletionSelection,
  selectedCompletionIndex,
  setSelectedCompletion,
  startCompletion,
  type Completion,
  type CompletionContext,
  type CompletionResult,
  type CompletionSource,
} from '@codemirror/autocomplete'
import { indentLess, insertTab } from '@codemirror/commands'
import {
  MySQL,
  PostgreSQL,
  SQLite,
  StandardSQL,
  keywordCompletionSource,
  sql,
  type SQLDialect,
} from '@codemirror/lang-sql'
import { Prec, type Extension } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
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
  onConnectionRequired?: () => void
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

// Keep Tab inside the SQL editor: accept a highlighted completion when one is
// active, otherwise fall back to standard Tab behavior — insertTab indents
// selected lines but inserts a literal tab/spaces at a bare cursor, unlike
// indentMore which always shifts the current line regardless of selection.
export function acceptCompletionOnTab(view: EditorView, event: KeyboardEvent): boolean {
  if (event.key !== 'Tab' || event.metaKey || event.ctrlKey || event.altKey) {
    return false
  }
  if (event.shiftKey) return indentLess(view)
  if (completionStatus(view.state) === 'active') {
    if (selectedCompletionIndex(view.state) === null) {
      view.dispatch({ effects: setSelectedCompletion(0) })
    }
    acceptCompletion(view)
    return true
  }
  return insertTab(view)
}

function closeCompletionAndKeepFocus(view: EditorView): boolean {
  if (completionStatus(view.state) === null) return false
  closeCompletion(view)
  view.focus()
  return true
}

const completionKeyboardHandler = EditorView.domEventHandlers({
  keydown(event, view) {
    if (event.key === 'Escape' && closeCompletionAndKeepFocus(view)) {
      event.preventDefault()
      event.stopPropagation()
      return true
    }
    return acceptCompletionOnTab(view, event)
  },
})

const completionNavigationKeymap = Prec.highest(
  keymap.of([
    { key: 'Ctrl-Space', run: startCompletion },
    { mac: 'Alt-`', run: startCompletion },
    { mac: 'Alt-i', run: startCompletion },
    { key: 'ArrowDown', run: moveCompletionSelection(true) },
    { key: 'ArrowUp', run: moveCompletionSelection(false) },
    { key: 'PageDown', run: moveCompletionSelection(true, 'page') },
    { key: 'PageUp', run: moveCompletionSelection(false, 'page') },
    // With selectOnOpen disabled this returns false for an untouched menu,
    // allowing CodeMirror's normal Enter binding to insert a newline.
    { key: 'Enter', run: acceptCompletion },
  ]),
)

const vocabularyCache = new Map<string, Promise<SQLCompletionSuggestion[]>>()
const IDENTIFIER_VALID_FOR = /^[\w$]*$/
const MATCH_TIER_BOOST = 1_000
const SEMANTIC_SPACE_KEYWORDS = new Set([
  'SELECT',
  'DISTINCT',
  'FROM',
  'JOIN',
  'UPDATE',
  'INTO',
  'USING',
  'LATERAL',
  'WHERE',
  'ON',
  'HAVING',
  'SET',
  'RETURNING',
  'AND',
  'OR',
  'WHEN',
  'THEN',
  'ELSE',
])

type SQLTriggerToken = {
  text: string
  kind: 'word' | 'identifier' | 'number' | 'symbol' | 'value'
  depth: number
}

type SQLTriggerScan = {
  tokens: SQLTriggerToken[]
  protectedRegion: boolean
}

function scanSQLTriggerPrefix(source: string): SQLTriggerScan {
  let tokens: SQLTriggerToken[] = []
  let depth = 0
  let i = 0

  const push = (text: string, kind: SQLTriggerToken['kind']) => {
    tokens.push({ text, kind, depth })
  }

  while (i < source.length) {
    const char = source[i]
    const next = source[i + 1]

    if (/\s/.test(char)) {
      i++
      continue
    }
    if (char === '-' && next === '-') {
      const newline = source.indexOf('\n', i + 2)
      if (newline === -1) return { tokens, protectedRegion: true }
      i = newline + 1
      continue
    }
    if (char === '/' && next === '*') {
      const end = source.indexOf('*/', i + 2)
      if (end === -1) return { tokens, protectedRegion: true }
      i = end + 2
      continue
    }
    if (char === "'") {
      i++
      let closed = false
      while (i < source.length) {
        if (source[i] !== "'") {
          i++
          continue
        }
        if (source[i + 1] === "'") {
          i += 2
          continue
        }
        i++
        closed = true
        break
      }
      if (!closed) return { tokens, protectedRegion: true }
      push('', 'value')
      continue
    }
    if (char === '"' || char === '`' || char === '[') {
      const closing = char === '[' ? ']' : char
      i++
      let closed = false
      while (i < source.length) {
        if (source[i] !== closing) {
          i++
          continue
        }
        if (closing !== ']' && source[i + 1] === closing) {
          i += 2
          continue
        }
        i++
        closed = true
        break
      }
      if (!closed) return { tokens, protectedRegion: true }
      push('', 'identifier')
      continue
    }
    if (char === '$') {
      const tag = source.slice(i).match(/^\$(?:[A-Za-z_][A-Za-z0-9_]*)?\$/)?.[0]
      if (tag) {
        const end = source.indexOf(tag, i + tag.length)
        if (end === -1) return { tokens, protectedRegion: true }
        i = end + tag.length
        push('', 'value')
        continue
      }
    }
    if (/[A-Za-z_]/.test(char)) {
      const start = i++
      while (i < source.length && /[A-Za-z0-9_$]/.test(source[i])) i++
      push(source.slice(start, i), 'word')
      continue
    }
    if (/[0-9]/.test(char)) {
      const start = i++
      while (i < source.length && /[0-9.eE+-]/.test(source[i])) i++
      push(source.slice(start, i), 'number')
      continue
    }
    if (char === ';') {
      tokens = []
      depth = 0
      i++
      continue
    }
    if (char === '(') {
      push(char, 'symbol')
      depth++
      i++
      continue
    }
    if (char === ')') {
      depth = Math.max(0, depth - 1)
      push(char, 'symbol')
      i++
      continue
    }
    push(char, 'symbol')
    i++
  }

  return { tokens, protectedRegion: false }
}

function previousWord(
  tokens: SQLTriggerToken[],
  before = tokens.length,
): SQLTriggerToken | undefined {
  for (let i = before - 1; i >= 0; i--) {
    if (tokens[i].kind === 'word') return tokens[i]
  }
  return undefined
}

function hasDMLContext(tokens: SQLTriggerToken[]): boolean {
  return tokens.some(
    (token) =>
      token.kind === 'word' &&
      ['SELECT', 'INSERT', 'UPDATE', 'DELETE'].includes(token.text.toLocaleUpperCase()),
  )
}

function hasSemanticIdentifierContext(source: string, cursor: number, prefix: string): boolean {
  if (prefix.length < 2) return false
  const scan = scanSQLTriggerPrefix(source.slice(0, cursor))
  return !scan.protectedRegion && hasDMLContext(scan.tokens)
}

export function automaticSQLCompletionTrigger(source: string, cursor: number): string | undefined {
  if (cursor <= 0 || cursor > source.length) return undefined
  const trigger = source[cursor - 1]
  if (!['.', ' ', ',', '('].includes(trigger)) return undefined
  if (trigger === ' ' && (cursor < 2 || /\s/.test(source[cursor - 2]))) return undefined

  const scan = scanSQLTriggerPrefix(source.slice(0, cursor))
  if (scan.protectedRegion || scan.tokens.length === 0) return undefined
  const last = scan.tokens[scan.tokens.length - 1]

  if (trigger === '.') {
    const owner = scan.tokens[scan.tokens.length - 2]
    return last.text === '.' &&
      owner !== undefined &&
      (owner.kind === 'word' || owner.kind === 'identifier')
      ? trigger
      : undefined
  }

  if (trigger === ' ') {
    if (last.kind !== 'word') return undefined
    const lastWord = last
    const keyword = lastWord.text.toLocaleUpperCase()
    if (keyword === 'BY') {
      const byIndex = scan.tokens.lastIndexOf(lastWord)
      const clause = previousWord(scan.tokens, byIndex)
      return clause && ['GROUP', 'ORDER', 'PARTITION'].includes(clause.text.toLocaleUpperCase())
        ? trigger
        : undefined
    }
    return SEMANTIC_SPACE_KEYWORDS.has(keyword) ? trigger : undefined
  }

  if (!hasDMLContext(scan.tokens)) return undefined
  if (trigger === ',') return last.text === ',' ? trigger : undefined
  if (trigger === '(') {
    const owner = scan.tokens[scan.tokens.length - 2]
    return last.text === '(' &&
      owner !== undefined &&
      (owner.kind === 'word' || owner.kind === 'identifier')
      ? trigger
      : undefined
  }
  return undefined
}

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

function completionMatchTier(label: string, prefix: string): number {
  const foldedLabel = label.toLocaleLowerCase()
  const foldedPrefix = prefix.toLocaleLowerCase()
  if (foldedPrefix.length === 0) return 0
  if (foldedLabel === foldedPrefix) return 5
  if (foldedLabel.startsWith(foldedPrefix)) return 4
  if (foldedLabel.split(/[^\p{L}\p{N}]+/u).some((segment) => segment.startsWith(foldedPrefix))) {
    return 3
  }
  if (foldedLabel.includes(foldedPrefix)) return 2
  const prefixCharacters = Array.from(foldedPrefix)
  if (prefixCharacters.length < 3) return 0
  let prefixIndex = 0
  for (const character of foldedLabel) {
    if (character === prefixCharacters[prefixIndex]) prefixIndex++
    if (prefixIndex === prefixCharacters.length) return 1
  }
  return 0
}

function rankSuggestions(
  suggestions: SQLCompletionSuggestion[],
  prefix: string,
): SQLCompletionSuggestion[] {
  if (prefix.length === 0) return suggestions
  return suggestions
    .map((suggestion) => ({ suggestion, tier: completionMatchTier(suggestion.label, prefix) }))
    .filter(({ tier }) => tier > 0)
    .sort(
      (left, right) =>
        right.tier - left.tier ||
        (right.suggestion.score ?? 0) - (left.suggestion.score ?? 0) ||
        left.suggestion.label.localeCompare(right.suggestion.label, undefined, {
          sensitivity: 'base',
        }),
    )
    .map(({ suggestion, tier }) => ({
      ...suggestion,
      score: tier * MATCH_TIER_BOOST + (suggestion.score ?? 0),
    }))
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

function mergeRankedCompletions(primary: Completion[], secondary: Completion[]): Completion[] {
  const merged = new Map<string, Completion>()
  for (const completion of [...primary, ...secondary]) {
    const key = `${completion.label.toLocaleLowerCase()}\u0000${completion.type ?? ''}`
    if (!merged.has(key)) merged.set(key, completion)
  }
  return [...merged.values()].sort(
    (left, right) =>
      (right.boost ?? 0) - (left.boost ?? 0) ||
      left.label.localeCompare(right.label, undefined, { sensitivity: 'base' }),
  )
}

export function remoteSQLCompletionSource(config: SQLCompletionConfig): CompletionSource {
  const dialect = dialectForDriver(config.driver)
  const localKeywords = keywordCompletionSource(dialect, true)
  let activeRemoteController: AbortController | undefined
  let remoteGeneration = 0

  return async (context: CompletionContext): Promise<CompletionResult | null> => {
    const word = context.matchBefore(/[A-Za-z_$][\w$]*$/)
    const prefix = word?.text ?? ''
    const source = context.state.doc.toString()
    const automaticTrigger = context.explicit
      ? undefined
      : automaticSQLCompletionTrigger(source, context.pos)
    const driver = normalizedDriver(config.driver)
    const supportsSemanticCompletion = driver !== 'sqlite' && driver !== 'standard'
    const hasRemote =
      supportsSemanticCompletion &&
      config.orgSlug !== undefined &&
      config.workspaceId !== undefined &&
      config.connectionId !== undefined &&
      config.driver !== undefined
    // Vocabulary is a last-resort prefix lookup, not a context-free menu.
    // In particular, Ctrl+Space at an empty prefix must not dump every dialect
    // keyword and function into an otherwise precise semantic result.
    const shouldCompleteLexically = prefix.length >= 2 || (context.explicit && prefix.length > 0)
    // A broad result from a semantic boundary (for example "HAVING ") may be
    // capped before the typed identifier exists. Re-run semantic completion
    // for a settled prefix instead of fuzzy-filtering that incomplete result.
    const shouldRetrySemanticIdentifier =
      !context.explicit && hasSemanticIdentifierContext(source, context.pos, prefix)
    const shouldCompleteRemotely =
      hasRemote &&
      (context.explicit || automaticTrigger !== undefined || shouldRetrySemanticIdentifier)

    if (context.explicit && config.connectionId === undefined) {
      config.onConnectionRequired?.()
    }

    let lexical: Completion[] = []
    if (shouldCompleteLexically) {
      try {
        const vocabulary = config.driver ? await loadVocabulary(config.driver) : []
        if (vocabulary.length === 0) {
          const fallback = await localKeywords(context)
          lexical = [...(fallback?.options ?? [])]
        } else {
          const foldedPrefix = prefix.toLocaleLowerCase()
          lexical = rankSuggestions(vocabulary, foldedPrefix).map(suggestionToCompletion)
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
        source,
        context.pos,
        config.sessionId,
        controller.signal,
        context.explicit ? 'invoked' : 'automatic',
        automaticTrigger,
      )
      if (context.aborted || controller.signal.aborted || generation !== remoteGeneration)
        return null
      const from = result.suggestions[0]?.replace_start ?? word?.from ?? context.pos
      const semantic = rankSuggestions(
        result.suggestions.filter((suggestion) => suggestion.replace_start === from),
        prefix,
      ).map(suggestionToCompletion)
      // Empty-prefix contexts remain semantic-only so broad vocabulary cannot
      // add noise. Once the user types a prefix, matching dialect vocabulary
      // may supplement parser candidates that omit valid functions or types.
      const options =
        semantic.length > 0
          ? prefix.length > 0
            ? mergeRankedCompletions(semantic, lexical)
            : semantic
          : lexical
      if (options.length === 0) return null
      return {
        from: Math.max(0, Math.min(from, context.pos)),
        to: context.pos,
        options,
        // Only reuse a semantic result for the exact prefix it was requested
        // with. The 150 ms activation delay and cancellation coalesce normal
        // typing while still letting the backend rank against the new prefix.
        validFor: (text) => text === prefix,
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
  return [
    sql({ dialect, upperCaseKeywords: true }),
    autocompletion({
      activateOnTypingDelay: 150,
      defaultKeymap: false,
      icons: false,
      interactionDelay: 0,
      selectOnOpen: false,
      addToOptions: [
        {
          position: 20,
          render: (completion) => renderCompletionIcon(completion, config.iconMap),
        },
      ],
      override: [remoteSQLCompletionSource(config)],
    }),
    completionTheme,
    completionKeyboardHandler,
    completionNavigationKeymap,
  ]
}
