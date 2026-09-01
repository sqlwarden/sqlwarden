import type { SQLCompletionSuggestion } from '#/lib/api/queries/database'
import type { CursorContext } from './context'
import type { CompletionIndex, IndexedColumn } from './schemaIndex'

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
        return ctx.cteNames.size > 0 ? 'backend' : 'local-only'
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
        return ctx.cteNames.has(target.table.toLowerCase()) ? 'backend' : 'local-only'
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

function objectSuggestion(o: {
  schema: string
  name: string
  kind: string
}): SQLCompletionSuggestion {
  return {
    label: o.name,
    kind: o.kind,
    namespace: o.schema || undefined,
    replace_start: 0,
    replace_end: 0,
    score: 60,
  }
}

function columnSuggestion(c: IndexedColumn): SQLCompletionSuggestion {
  return {
    label: c.name,
    kind: 'column',
    qualifier: c.table,
    namespace: c.schema || undefined,
    data_type: c.type,
    replace_start: 0,
    replace_end: 0,
    score: 70,
  }
}

export function resolveLocalCompletions(
  ctx: CursorContext,
  index: CompletionIndex,
): SQLCompletionSuggestion[] {
  if (ctx.protectedRegion) return []
  if (ctx.positionClass === 'value') return []

  if (ctx.positionClass === 'qualified') {
    const target = resolveAliasTable(ctx)
    if (!target) return []
    const prefix = ctx.prefix.toLowerCase()
    return columnsFor(index, target.table, target.schema)
      .filter((c) => !prefix || c.name.toLowerCase().startsWith(prefix))
      .map(columnSuggestion)
  }

  if (ctx.positionClass === 'relation') {
    return index.objects.map(objectSuggestion)
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
        out.push(columnSuggestion(col))
      }
    }
    return out
  }

  return [...index.objects.map(objectSuggestion), ...index.allColumns.map(columnSuggestion)]
}
