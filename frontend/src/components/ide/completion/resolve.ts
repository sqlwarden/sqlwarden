import type { SQLCompletionSuggestion } from '#/lib/api/queries/database'
import type { SqlDialect } from '../dialect'
import type { CursorContext } from './context'
import type { CompletionIndex, IndexedColumn, IndexedObject } from './schemaIndex'

export type CompletionPath = 'local-only' | 'local-then-backend' | 'backend'

function resolveAliasTable(ctx: CursorContext): { table: string; schema?: string } | undefined {
  if (!ctx.qualifier) return undefined
  const q = ctx.qualifier.toLowerCase()
  const byAlias = ctx.fromRefs.find((r) => r.alias?.toLowerCase() === q)
  if (byAlias) return { table: byAlias.table, schema: byAlias.schema }
  const byName = ctx.fromRefs.find((r) => r.table.toLowerCase() === q)
  if (byName) return { table: byName.table, schema: byName.schema }
  return undefined
}

function columnsFor(index: CompletionIndex, table: string, schema?: string): IndexedColumn[] {
  return (
    (schema && index.columnsByTable.get(`${schema.toLowerCase()} ${table.toLowerCase()}`)) ||
    index.columnsByTable.get(table.toLowerCase()) ||
    []
  )
}

function relationCovered(ctx: CursorContext, index: CompletionIndex): boolean {
  if (!index.defaultScopeRelationsListed) return false
  const prefix = ctx.prefix.toLowerCase()
  if (!prefix) return true
  const search = new Set(index.searchSchemas.map((schema) => schema.toLowerCase()))
  return index.objects.some(
    (o) => search.has(o.schema.toLowerCase()) && o.name.toLowerCase().startsWith(prefix),
  )
}

// An unqualified name resolves in the first search schema that holds it, so
// the same name in a later search schema is unreachable without a qualifier.
function searchVisibleObjects(index: CompletionIndex): IndexedObject[] {
  const rank = new Map<string, number>()
  index.searchSchemas.forEach((schema, i) => {
    const key = schema.toLowerCase()
    if (!rank.has(key)) rank.set(key, i)
  })
  const best = new Map<string, number>()
  for (const o of index.objects) {
    const r = rank.get(o.schema.toLowerCase())
    if (r === undefined) continue
    const name = o.name.toLowerCase()
    best.set(name, Math.min(best.get(name) ?? r, r))
  }
  return index.objects.filter((o) => {
    const r = rank.get(o.schema.toLowerCase())
    return r === undefined || r === best.get(o.name.toLowerCase())
  })
}

export function decideCompletionPath(
  ctx: CursorContext,
  index: CompletionIndex | null,
  explicit: boolean,
): CompletionPath {
  if (ctx.protectedRegion) return 'backend'
  if (!index) return 'backend'

  const localOnlyPath = (): CompletionPath => {
    switch (ctx.positionClass) {
      case 'relation':
        // A CTE name never lives in the persisted schema index, so the local
        // index can't answer a relation position honestly when one is defined.
        if (ctx.cteNames.size > 0) return 'backend'
        // Folders the user never expanded are absent from the index; only the
        // backend can fetch them, so the position is covered only when the
        // default scope's relation folders are listed and, for a typed prefix,
        // the default schema holds a match.
        return relationCovered(ctx, index) ? 'local-only' : 'backend'
      case 'value':
        // Only the backend classifies value expressions; the local index has
        // nothing correct to offer between VALUES parentheses.
        return 'backend'
      case 'keyword':
      case 'unknown':
        return 'local-only'
      case 'qualified': {
        const target = resolveAliasTable(ctx)
        if (!target) return 'backend'
        if (ctx.cteNames.has(target.table.toLowerCase())) return 'backend'
        return columnsFor(index, target.table, target.schema).length > 0 ? 'local-only' : 'backend'
      }
      case 'column': {
        const resolvable =
          ctx.fromRefs.length > 0 &&
          ctx.fromRefs.every(
            (r) =>
              !ctx.cteNames.has(r.table.toLowerCase()) &&
              columnsFor(index, r.table, r.schema).length > 0,
          )
        return resolvable ? 'local-only' : 'backend'
      }
      default:
        return 'backend'
    }
  }

  const path = localOnlyPath()
  // A backend-only position can't be downgraded by an explicit invoke — the
  // local index has nothing correct to offer there. Only a genuinely
  // local-answerable position benefits from also warming the backend.
  return explicit && path === 'local-only' ? 'local-then-backend' : path
}

function insertTextFor(
  name: string,
  dialect: SqlDialect | undefined,
): Pick<SQLCompletionSuggestion, 'insert_text'> {
  const quoted = dialect?.formatIdentifier(name)
  return quoted !== undefined && quoted !== name ? { insert_text: quoted } : {}
}

function objectSuggestion(
  o: IndexedObject,
  dialect: SqlDialect | undefined,
): SQLCompletionSuggestion {
  return {
    label: o.name,
    ...insertTextFor(o.name, dialect),
    kind: o.kind,
    namespace: o.schema || undefined,
    replace_start: 0,
    replace_end: 0,
    score: o.score,
  }
}

function columnSuggestion(
  c: IndexedColumn,
  score: number,
  dialect: SqlDialect | undefined,
): SQLCompletionSuggestion {
  return {
    label: c.name,
    ...insertTextFor(c.name, dialect),
    kind: 'column',
    qualifier: c.table,
    namespace: c.schema || undefined,
    data_type: c.type,
    replace_start: 0,
    replace_end: 0,
    score,
  }
}

export function resolveLocalCompletions(
  ctx: CursorContext,
  index: CompletionIndex,
  dialect?: SqlDialect,
): SQLCompletionSuggestion[] {
  if (ctx.protectedRegion) return []
  if (ctx.positionClass === 'value') return []
  // After a complete table reference or a clause lead such as ORDER, only
  // keywords or an alias can follow; other keyword positions (a statement
  // start, an identifier typed above an existing FROM) still want objects.
  if (ctx.positionClass === 'keyword' && (ctx.afterTableRef || ctx.afterClauseLead)) return []
  const toColumn = (c: IndexedColumn) => columnSuggestion(c, index.columnScore, dialect)

  if (ctx.positionClass === 'qualified') {
    const target = resolveAliasTable(ctx)
    if (!target) return []
    const prefix = ctx.prefix.toLowerCase()
    return columnsFor(index, target.table, target.schema)
      .filter((c) => !prefix || c.name.toLowerCase().startsWith(prefix))
      .map(toColumn)
  }

  if (ctx.positionClass === 'relation') {
    return searchVisibleObjects(index).map((o) => objectSuggestion(o, dialect))
  }

  if (ctx.positionClass === 'column' && ctx.fromRefs.length > 0) {
    const seen = new Set<string>()
    const out: SQLCompletionSuggestion[] = []
    for (const ref of ctx.fromRefs) {
      if (ctx.cteNames.has(ref.table.toLowerCase())) continue
      for (const col of columnsFor(index, ref.table, ref.schema)) {
        const key = `${col.table} ${col.name}`.toLowerCase()
        if (seen.has(key)) continue
        seen.add(key)
        out.push(toColumn(col))
      }
    }
    return out
  }

  return [
    ...index.objects.map((o) => objectSuggestion(o, dialect)),
    ...index.allColumns.map(toColumn),
  ]
}
