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
import { Prec, type EditorState, type Extension } from '@codemirror/state'
import { EditorView, keymap } from '@codemirror/view'
import { completeConnectionSQL } from '#/lib/api/queries/database'
import { findFrontendEngine } from '../engines/registry'
import {
  automaticSQLCompletionTrigger,
  classifyCursorContext,
  hasSemanticIdentifierContext,
  type CursorContext,
} from './context'
import {
  mergeRankedCompletions,
  rankSuggestions,
  suggestionToCompletion,
  type RankPositionHint,
} from './rank'
import { completionTheme, renderCompletionRow, type IconMap } from './render'
import { decideCompletionPath, resolveLocalCompletions } from './resolve'
import { getCompletionIndex } from './schemaIndex'
import { loadVocabulary, normalizedDriver } from './vocabularyCache'

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

const IDENTIFIER_VALID_FOR = /^[\w$]*$/

const RESPONSE_HINTS = new Set<RankPositionHint>(['column', 'relation', 'value', 'keyword', 'any'])

function positionHint(positionClass: CursorContext['positionClass']): RankPositionHint {
  switch (positionClass) {
    case 'relation':
      return 'relation'
    case 'qualified':
    case 'column':
      return 'column'
    case 'value':
      return 'value'
    case 'keyword':
      return 'keyword'
    default:
      return 'any'
  }
}

function responseHint(context: string | undefined, fallback: RankPositionHint): RankPositionHint {
  return context !== undefined && RESPONSE_HINTS.has(context as RankPositionHint)
    ? (context as RankPositionHint)
    : fallback
}

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
    const supportsSemanticCompletion =
      findFrontendEngine(normalizedDriver(config.driver))?.semanticCompletion === true
    const hasRemote =
      supportsSemanticCompletion &&
      config.orgSlug !== undefined &&
      config.workspaceId !== undefined &&
      config.connectionId !== undefined &&
      config.driver !== undefined

    if (context.explicit && config.connectionId === undefined) {
      config.onConnectionRequired?.()
    }

    const cursorContext = classifyCursorContext(source, context.pos)
    if (cursorContext.protectedRegion) return null

    // Warm both client-side sources without blocking; both are memoised so a
    // repeat completion in the same session issues no extra round trip.
    const indexPromise = getCompletionIndex(config)
    const vocabPromise = config.driver ? loadVocabulary(config.driver) : Promise.resolve([])

    // Vocabulary is a last-resort prefix lookup, not a context-free menu.
    // In particular, Ctrl+Space at an empty prefix must not dump every dialect
    // keyword and function into an otherwise precise semantic result.
    const shouldCompleteLexically = prefix.length >= 2 || (context.explicit && prefix.length > 0)
    // A broad result from a semantic boundary (for example "HAVING ") may be
    // capped before the typed identifier exists. Re-run semantic completion
    // for a settled prefix instead of fuzzy-filtering that incomplete result.
    const shouldRetrySemanticIdentifier =
      !context.explicit && hasSemanticIdentifierContext(source, context.pos, prefix)

    let lexical: Completion[] = []
    if (shouldCompleteLexically) {
      try {
        const vocabulary = await vocabPromise
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

    const index = await indexPromise
    const path = decideCompletionPath(cursorContext, index, context.explicit)
    const localHint = positionHint(cursorContext.positionClass)

    const from = word?.from ?? context.pos
    const resolvedLocal = index ? resolveLocalCompletions(cursorContext, index) : []
    const localRanked = rankSuggestions(
      resolvedLocal.map((suggestion) => ({
        ...suggestion,
        replace_start: from,
        replace_end: context.pos,
      })),
      prefix,
      localHint,
    ).map(suggestionToCompletion)
    const local = mergeRankedCompletions(localRanked, lexical)

    // Below the lexical threshold the local result is deliberately incomplete
    // (dialect vocabulary is withheld until the prefix settles), so CodeMirror
    // must re-invoke the source as the word grows rather than fuzzy-filter the
    // object-only rows it already has. Once vocabulary is in play the result is
    // complete for the word and cheap identifier refiltering is correct.
    const localValidFor = shouldCompleteLexically
      ? IDENTIFIER_VALID_FOR
      : (text: string) => text === prefix
    const localResult = (options: Completion[]): CompletionResult | null => {
      if (options.length === 0) return null
      return {
        from,
        to: context.pos,
        options,
        validFor: localValidFor,
      }
    }

    if (path === 'local-only') return localResult(local)

    const wantBackend =
      hasRemote &&
      (context.explicit || automaticTrigger !== undefined || shouldRetrySemanticIdentifier)

    // A warm local index answers relation and keyword positions completely, so
    // an explicit invoke there skips the round trip. Column and qualified
    // positions still consult the backend: only it sees derived-table and CTE
    // columns, alias scoping, and full function signatures the local index lacks.
    const localAnswersPositionFully =
      cursorContext.positionClass !== 'column' && cursorContext.positionClass !== 'qualified'
    if (path === 'local-then-backend' && resolvedLocal.length > 0 && localAnswersPositionFully) {
      return localResult(local)
    }

    if (!wantBackend) return localResult(local)

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
      const remoteFrom = result.suggestions[0]?.replace_start ?? from
      const semantic = rankSuggestions(
        result.suggestions.filter((suggestion) => suggestion.replace_start === remoteFrom),
        prefix,
        responseHint(result.context, localHint),
      ).map(suggestionToCompletion)
      // Empty-prefix contexts remain semantic-only so broad vocabulary cannot
      // add noise. Once the user types a prefix, matching dialect vocabulary
      // and warm local rows may supplement parser candidates that omit valid
      // functions, types, or objects.
      const options =
        semantic.length > 0
          ? prefix.length > 0
            ? mergeRankedCompletions(semantic, local)
            : semantic
          : local.length > 0
            ? local
            : lexical
      if (options.length === 0) return null
      return {
        from: Math.max(0, Math.min(remoteFrom, context.pos)),
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
      if (local.length > 0) return localResult(local)
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
  // Warm the client caches so the first completion in an editor is local-first.
  void getCompletionIndex(config)
  if (config.driver) void loadVocabulary(config.driver)
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
          position: 50,
          // CodeMirror passes the fuzzy-match ranges as a 4th argument its
          // published type omits; the optional parameter keeps this
          // assignable while letting the row highlight the matched substring.
          render: (
            completion: Completion,
            _state: EditorState,
            _view: EditorView,
            match?: readonly number[],
          ) => renderCompletionRow(completion, config.iconMap, match),
        },
      ],
      override: [remoteSQLCompletionSource(config)],
    }),
    completionTheme,
    completionKeyboardHandler,
    completionNavigationKeymap,
  ]
}
