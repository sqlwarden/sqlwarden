import {
  getConnectionCompletionIndex,
  type SQLCompletionIndexResponse,
} from '#/lib/api/queries/database'
import type { SQLCompletionConfig } from './source'

export type IndexedObject = { schema: string; name: string; kind: string; score: number }
export type IndexedColumn = {
  schema: string
  table: string
  name: string
  type?: string
  nullable?: boolean
}
export type CompletionIndex = {
  version: string
  defaultSchema: string
  searchSchemas: string[]
  defaultScopeRelationsListed: boolean
  columnScore: number
  schemas: string[]
  objects: IndexedObject[]
  columnsByTable: Map<string, IndexedColumn[]>
  allColumns: IndexedColumn[]
}

type Entry = { promise: Promise<CompletionIndex | null>; failedAt?: number }

const RETRY_BACKOFF_MS = 30_000
const cache = new Map<number, Entry>()

function fullyIdentified(
  config: SQLCompletionConfig,
): config is SQLCompletionConfig & { orgSlug: string; workspaceId: number; connectionId: number } {
  return (
    config.orgSlug !== undefined &&
    config.workspaceId !== undefined &&
    config.connectionId !== undefined
  )
}

function buildIndex(payload: SQLCompletionIndexResponse): CompletionIndex {
  const columnsByTable = new Map<string, IndexedColumn[]>()
  const allColumns: IndexedColumn[] = []
  for (const raw of payload.columns) {
    const column: IndexedColumn = {
      schema: raw.schema,
      table: raw.table,
      name: raw.name,
      type: raw.type,
      nullable: raw.nullable,
    }
    allColumns.push(column)
    for (const key of [
      `${raw.schema.toLowerCase()} ${raw.table.toLowerCase()}`,
      raw.table.toLowerCase(),
    ]) {
      const bucket = columnsByTable.get(key)
      if (bucket) bucket.push(column)
      else columnsByTable.set(key, [column])
    }
  }
  return {
    version: payload.version,
    defaultSchema: payload.default_schema,
    searchSchemas: payload.search_schemas,
    defaultScopeRelationsListed: payload.default_scope_relations_listed,
    columnScore: payload.column_score,
    schemas: payload.schemas,
    objects: payload.objects.map((o) => ({
      schema: o.schema,
      name: o.name,
      kind: o.kind,
      score: o.score,
    })),
    columnsByTable,
    allColumns,
  }
}

export function getCompletionIndex(config: SQLCompletionConfig): Promise<CompletionIndex | null> {
  if (!fullyIdentified(config)) return Promise.resolve(null)
  const key = config.connectionId
  const existing = cache.get(key)
  if (existing) {
    if (existing.failedAt === undefined || Date.now() - existing.failedAt < RETRY_BACKOFF_MS) {
      return existing.promise
    }
    cache.delete(key)
  }

  const entry: Entry = {
    promise: getConnectionCompletionIndex(
      config.orgSlug,
      config.workspaceId,
      config.connectionId,
      config.sessionId,
    )
      .then(buildIndex)
      .catch(() => {
        const current = cache.get(key)
        if (current) current.failedAt = Date.now()
        return null
      }),
  }
  cache.set(key, entry)
  return entry.promise
}

export function invalidateCompletionIndex(connectionId: number): void {
  cache.delete(connectionId)
}

export function clearCompletionIndexCache(): void {
  cache.clear()
}
