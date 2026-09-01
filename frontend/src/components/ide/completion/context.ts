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

export type SQLTriggerToken = {
  text: string
  kind: 'word' | 'identifier' | 'number' | 'symbol' | 'value'
  depth: number
}

export type SQLTriggerScan = {
  tokens: SQLTriggerToken[]
  protectedRegion: boolean
  depth: number
}

export function scanSQLTriggerPrefix(source: string): SQLTriggerScan {
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
      if (newline === -1) return { tokens, protectedRegion: true, depth }
      i = newline + 1
      continue
    }
    if (char === '/' && next === '*') {
      const end = source.indexOf('*/', i + 2)
      if (end === -1) return { tokens, protectedRegion: true, depth }
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
      if (!closed) return { tokens, protectedRegion: true, depth }
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
      if (!closed) return { tokens, protectedRegion: true, depth }
      push('', 'identifier')
      continue
    }
    if (char === '$') {
      const tag = source.slice(i).match(/^\$(?:[A-Za-z_][A-Za-z0-9_]*)?\$/)?.[0]
      if (tag) {
        const end = source.indexOf(tag, i + tag.length)
        if (end === -1) return { tokens, protectedRegion: true, depth }
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
      // A bare statement terminator resets the scan, but a ';' nested in a
      // parenthesised group (e.g. a routine body) is not a boundary.
      if (depth === 0) {
        tokens = []
      } else {
        push(char, 'symbol')
      }
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

  return { tokens, protectedRegion: false, depth }
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

export function hasSemanticIdentifierContext(
  source: string,
  cursor: number,
  prefix: string,
): boolean {
  if (prefix.length < 2) return false
  const scan = scanSQLTriggerPrefix(source.slice(0, cursor))
  return !scan.protectedRegion && hasDMLContext(scan.tokens)
}

export type CursorRelationRef = { table: string; schema?: string; alias?: string }

export type CursorContext = {
  positionClass: 'qualified' | 'relation' | 'column' | 'value' | 'keyword' | 'unknown'
  qualifier?: string
  fromRefs: CursorRelationRef[]
  cteNames: Set<string>
  prefix: string
  protectedRegion: boolean
}

const RELATION_GOV = new Set(['FROM', 'JOIN', 'INTO', 'UPDATE'])
const COLUMN_GOV = new Set([
  'SELECT',
  'DISTINCT',
  'WHERE',
  'ON',
  'HAVING',
  'SET',
  'BY',
  'AND',
  'OR',
  'RETURNING',
  'VALUES',
  'USING',
])
const KEYWORD_GOV = new Set(['GROUP', 'ORDER'])
const GOV_KEYWORDS = new Set([...RELATION_GOV, ...COLUMN_GOV, ...KEYWORD_GOV])

const REF_LIST_STOP = new Set([
  'WHERE',
  'GROUP',
  'ORDER',
  'HAVING',
  'LIMIT',
  'WINDOW',
  'UNION',
  'EXCEPT',
  'INTERSECT',
  'RETURNING',
  'ON',
  'USING',
  'SET',
  'VALUES',
])
const JOIN_LEADS = new Set([
  'INNER',
  'LEFT',
  'RIGHT',
  'FULL',
  'OUTER',
  'CROSS',
  'NATURAL',
  'LATERAL',
])
const NOT_ALIAS = new Set([...REF_LIST_STOP, ...JOIN_LEADS, 'JOIN', 'FROM', 'AS'])

function currentStatementBounds(source: string, cursor: number): { start: number; end: number } {
  let start = 0
  let i = 0
  const n = source.length
  while (i < n) {
    const char = source[i]
    const next = source[i + 1]
    if (char === '-' && next === '-') {
      const newline = source.indexOf('\n', i + 2)
      if (newline === -1) return { start, end: n }
      i = newline + 1
      continue
    }
    if (char === '/' && next === '*') {
      const close = source.indexOf('*/', i + 2)
      if (close === -1) return { start, end: n }
      i = close + 2
      continue
    }
    if (char === "'" || char === '"' || char === '`') {
      const quote = char
      i++
      while (i < n) {
        if (source[i] === quote) {
          if (source[i + 1] === quote) {
            i += 2
            continue
          }
          i++
          break
        }
        i++
      }
      continue
    }
    if (char === ';') {
      if (i >= cursor) return { start, end: i }
      start = i + 1
      i++
      continue
    }
    i++
  }
  return { start, end: n }
}

function wordUpper(token: SQLTriggerToken | undefined): string {
  return token && token.kind === 'word' ? token.text.toLocaleUpperCase() : ''
}

// consumeParenGroup advances past a balanced parenthesized group starting at
// an opening '(' token, returning the index just after the matching ')'.
function consumeParenGroup(tokens: SQLTriggerToken[], openIndex: number): number {
  let depth = 0
  let i = openIndex
  while (i < tokens.length) {
    const token = tokens[i]
    if (token.kind === 'symbol' && token.text === '(') depth++
    else if (token.kind === 'symbol' && token.text === ')') {
      depth--
      if (depth === 0) return i + 1
    }
    i++
  }
  return i
}

// collectCteNamesInto parses the "name [(cols)] AS ( body )[, ...]" list that
// follows a WITH keyword, adding each bound name to the set.
function collectCteNamesInto(tokens: SQLTriggerToken[], start: number, names: Set<string>): void {
  let i = start
  if (wordUpper(tokens[i]) === 'RECURSIVE') i++
  while (i < tokens.length) {
    const nameToken = tokens[i]
    if (nameToken.kind !== 'word') break
    let j = i + 1
    if (tokens[j]?.kind === 'symbol' && tokens[j]?.text === '(') {
      j = consumeParenGroup(tokens, j)
    }
    if (wordUpper(tokens[j]) !== 'AS') break
    j++
    if (tokens[j]?.kind !== 'symbol' || tokens[j]?.text !== '(') break
    names.add(nameToken.text.toLocaleLowerCase())
    i = consumeParenGroup(tokens, j)
    if (tokens[i]?.kind === 'symbol' && tokens[i]?.text === ',') {
      i++
      continue
    }
    break
  }
}

// extractCteNames finds every name bound by a WITH clause — the leading one and
// any nested inside a subquery — so callers can tell a CTE reference apart from
// a persisted schema table. CTE columns and even the CTE name itself never
// appear in the local schema index. Names are collected without scope tracking;
// callers only use the set to steer an ambiguous slot toward the backend.
function extractCteNames(tokens: SQLTriggerToken[]): Set<string> {
  const names = new Set<string>()
  for (let i = 0; i < tokens.length; i++) {
    const token = tokens[i]
    if (token.kind !== 'word' || wordUpper(token) !== 'WITH') continue
    const previous = tokens[i - 1]
    if (i === 0 || (previous?.kind === 'symbol' && previous.text === '(')) {
      collectCteNamesInto(tokens, i + 1, names)
    }
  }
  return names
}

// insertColumnListRef detects the "INSERT INTO <table> ( col, ... " column
// list and returns the target relation, so the slot resolves that table's
// columns instead of being mistaken for a relation position.
function insertColumnListRef(gov: string, post: SQLTriggerToken[]): CursorRelationRef | undefined {
  if (gov !== 'INTO' || post[0]?.kind !== 'word') return undefined
  let i = 0
  let schema: string | undefined
  let table = post[i].text
  i++
  if (post[i]?.kind === 'symbol' && post[i]?.text === '.' && post[i + 1]?.kind === 'word') {
    schema = table
    table = post[i + 1].text
    i += 2
  }
  if (post[i]?.kind !== 'symbol' || post[i]?.text !== '(') return undefined
  let depth = 0
  for (let j = i; j < post.length; j++) {
    const token = post[j]
    if (token.kind === 'symbol' && token.text === '(') depth++
    else if (token.kind === 'symbol' && token.text === ')') depth--
    else if (
      token.kind === 'word' &&
      (wordUpper(token) === 'VALUES' || wordUpper(token) === 'SELECT')
    ) {
      return undefined
    }
  }
  if (depth <= 0) return undefined
  return { table, ...(schema ? { schema } : {}) }
}

function extractRelationRefs(tokens: SQLTriggerToken[], targetDepth: number): CursorRelationRef[] {
  const refs: CursorRelationRef[] = []
  let i = 0
  while (i < tokens.length) {
    const token = tokens[i]
    const upper = wordUpper(token)
    if (
      token.kind === 'word' &&
      (upper === 'FROM' || upper === 'JOIN') &&
      token.depth === targetDepth
    ) {
      const baseDepth = token.depth
      const single = upper === 'JOIN'
      i++
      let expectRef = true
      while (i < tokens.length) {
        const current = tokens[i]
        if (current.kind === 'symbol' && current.text === '(' && expectRef) {
          // A derived table / subquery in the reference list: skip the whole
          // group and its optional alias without recording a phantom relation.
          i = consumeParenGroup(tokens, i)
          const aliasToken = tokens[i]
          if (
            aliasToken?.kind === 'word' &&
            wordUpper(aliasToken) === 'AS' &&
            tokens[i + 1]?.kind === 'word'
          ) {
            i += 2
          } else if (aliasToken?.kind === 'word' && !NOT_ALIAS.has(wordUpper(aliasToken))) {
            i++
          }
          expectRef = false
          if (single) break
          continue
        }
        if (current.depth > baseDepth) {
          i++
          continue
        }
        const currentUpper = wordUpper(current)
        if (current.kind === 'word' && (currentUpper === 'FROM' || currentUpper === 'JOIN')) break
        if (current.kind === 'word' && REF_LIST_STOP.has(currentUpper)) break
        if (current.kind === 'symbol' && current.text === ',') {
          expectRef = true
          i++
          continue
        }
        if (current.kind !== 'word' || !expectRef || GOV_KEYWORDS.has(currentUpper)) {
          i++
          continue
        }

        let schema: string | undefined
        let table = current.text
        i++
        if (
          tokens[i]?.kind === 'symbol' &&
          tokens[i]?.text === '.' &&
          tokens[i + 1]?.kind === 'word'
        ) {
          schema = table
          table = tokens[i + 1].text
          i += 2
        }

        let alias: string | undefined
        const aliasToken = tokens[i]
        if (
          aliasToken?.kind === 'word' &&
          wordUpper(aliasToken) === 'AS' &&
          tokens[i + 1]?.kind === 'word'
        ) {
          alias = tokens[i + 1].text
          i += 2
        } else if (aliasToken?.kind === 'word' && !NOT_ALIAS.has(wordUpper(aliasToken))) {
          alias = aliasToken.text
          i++
        }

        refs.push({
          table,
          ...(schema ? { schema } : {}),
          ...(alias ? { alias } : {}),
        })
        expectRef = false
        if (single) break
      }
      continue
    }
    i++
  }
  return refs
}

export function classifyCursorContext(source: string, cursor: number): CursorContext {
  const { start, end } = currentStatementBounds(source, cursor)
  const statement = source.slice(start, end)
  const statementCursor = Math.max(0, Math.min(cursor - start, statement.length))

  const beforeText = statement.slice(0, statementCursor)
  const beforeScan = scanSQLTriggerPrefix(beforeText)
  if (beforeScan.protectedRegion) {
    return {
      positionClass: 'unknown',
      fromRefs: [],
      cteNames: new Set(),
      prefix: '',
      protectedRegion: true,
    }
  }

  const statementTokens = scanSQLTriggerPrefix(statement).tokens
  const cteNames = extractCteNames(statementTokens)
  const fromRefs = extractRelationRefs(statementTokens, beforeScan.depth)
  const endsWithSpace = /\s$/.test(beforeText)
  const prefix = endsWithSpace ? '' : (beforeText.match(/[A-Za-z_$][\w$]*$/)?.[0] ?? '')

  const tokens = beforeScan.tokens
  const lastIsPrefixWord =
    prefix !== '' &&
    tokens.length > 0 &&
    tokens[tokens.length - 1].kind === 'word' &&
    tokens[tokens.length - 1].text === prefix
  const core = lastIsPrefixWord ? tokens.slice(0, -1) : tokens

  const qualifier = qualifierBeforeCursor(core, prefix, endsWithSpace)
  if (qualifier !== undefined) {
    return {
      positionClass: 'qualified',
      qualifier,
      fromRefs,
      cteNames,
      prefix,
      protectedRegion: false,
    }
  }

  let govIndex = -1
  for (let i = core.length - 1; i >= 0; i--) {
    if (core[i].kind === 'word' && GOV_KEYWORDS.has(core[i].text.toLocaleUpperCase())) {
      govIndex = i
      break
    }
  }

  if (govIndex === -1) {
    return {
      positionClass: prefix ? 'keyword' : 'unknown',
      fromRefs,
      cteNames,
      prefix,
      protectedRegion: false,
    }
  }

  const gov = core[govIndex].text.toLocaleUpperCase()
  const post = core.slice(govIndex + 1)

  if (RELATION_GOV.has(gov)) {
    const insertRef = insertColumnListRef(gov, post)
    if (insertRef) {
      return {
        positionClass: 'column',
        fromRefs: [insertRef],
        cteNames,
        prefix,
        protectedRegion: false,
      }
    }
    const lastPost = post[post.length - 1]
    if (post.length === 0) return relation(fromRefs, cteNames, prefix)
    if (lastPost.kind === 'symbol' && (lastPost.text === ',' || lastPost.text === '.')) {
      return relation(fromRefs, cteNames, prefix)
    }
    if (!prefix && !endsWithSpace) return relation(fromRefs, cteNames, prefix)
    return keyword(fromRefs, cteNames, prefix)
  }

  if (COLUMN_GOV.has(gov)) {
    if (gov === 'VALUES') {
      return { positionClass: 'value', fromRefs, cteNames, prefix, protectedRegion: false }
    }
    return { positionClass: 'column', fromRefs, cteNames, prefix, protectedRegion: false }
  }

  return keyword(fromRefs, cteNames, prefix)
}

function relation(
  fromRefs: CursorRelationRef[],
  cteNames: Set<string>,
  prefix: string,
): CursorContext {
  return { positionClass: 'relation', fromRefs, cteNames, prefix, protectedRegion: false }
}

function keyword(
  fromRefs: CursorRelationRef[],
  cteNames: Set<string>,
  prefix: string,
): CursorContext {
  return { positionClass: 'keyword', fromRefs, cteNames, prefix, protectedRegion: false }
}

function qualifierBeforeCursor(
  core: SQLTriggerToken[],
  prefix: string,
  endsWithSpace: boolean,
): string | undefined {
  if (endsWithSpace) return undefined
  const last = core[core.length - 1]
  if (prefix !== '') {
    // core already has the prefix word stripped; expect "... word ."
    if (last?.kind === 'symbol' && last.text === '.') {
      const owner = core[core.length - 2]
      if (owner && (owner.kind === 'word' || owner.kind === 'identifier')) return owner.text
    }
    return undefined
  }
  if (last?.kind === 'symbol' && last.text === '.') {
    const owner = core[core.length - 2]
    if (owner && (owner.kind === 'word' || owner.kind === 'identifier')) return owner.text
  }
  return undefined
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
