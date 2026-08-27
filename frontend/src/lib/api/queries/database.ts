import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import type {
  DirectoryResponse,
  EngineView,
  GenerateStatementResponse,
  ObjectRef,
  ObjectsResponse,
  RelationshipsResponse,
  ResultSet,
  SchemaEditRequest,
  SchemaEditResponse,
  SchemaRefreshResponse,
  SchemaSpecResponse,
  StatementOperation,
  TransactionMode,
  TransactionStatusResponse,
} from '#/lib/api/types'
import { queryKeys } from '#/lib/api/query-keys'

export function connectionDirectoryQueryKey(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
) {
  return ['connection-directory', slug, String(workspaceId), String(connectionId)] as const
}

export function connectionSchemaSpecQueryKey(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
) {
  return ['connection-schema-spec', slug, String(workspaceId), String(connectionId)] as const
}

function schemaBase(slug: string, workspaceId: string | number, connectionId: string | number) {
  return `/api/v1/orgs/${slug}/workspaces/${workspaceId}/connections/${connectionId}/schema`
}

function schemaRequestOptions(sessionId?: string) {
  return sessionId ? { headers: { 'X-Warden-Session': sessionId } } : undefined
}

export type SQLCompletionSuggestion = {
  label: string
  display_label?: string
  kind: string
  detail?: string
  insert_text?: string
  replace_start: number
  replace_end: number
  score?: number
}

export type SQLCompletionVocabulary = {
  dialect: string
  version: string
  suggestions: SQLCompletionSuggestion[]
}

export type SQLCompletionResponse = {
  suggestions: SQLCompletionSuggestion[]
  mode: 'persistent' | 'ephemeral'
  metadata_available: boolean
  metadata_status: string
  snapshot_id?: string
}

export function completeConnectionSQL(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sql: string,
  cursorOffset: number,
  sessionId: string | undefined,
  signal: AbortSignal,
  triggerKind: 'invoked' | 'automatic' = 'invoked',
  triggerCharacter?: string,
) {
  return api.post<SQLCompletionResponse>(
    `/api/v1/orgs/${slug}/workspaces/${workspaceId}/connections/${connectionId}/completion`,
    {
      sql,
      cursor_offset: cursorOffset,
      trigger_kind: triggerKind,
      ...(triggerCharacter ? { trigger_character: triggerCharacter } : {}),
    },
    {
      signal,
      ...(sessionId ? { headers: { 'X-Warden-Session': sessionId } } : {}),
    },
  )
}

/** Connection drivers use their driver library's name; the engine registry
 *  keys capabilities by SQLWarden's own engine id. */
function normalizeEngineID(driver: string): string {
  return driver === 'postgresql'
    ? 'postgres'
    : driver === 'mariadb'
      ? 'mysql'
      : driver === 'sqlite3'
        ? 'sqlite'
        : driver
}

export function getSQLCompletionVocabulary(driver: string, signal?: AbortSignal) {
  return api.get<SQLCompletionVocabulary>(
    `/api/v1/engines/${normalizeEngineID(driver)}/completion-vocabulary`,
    signal ? { signal } : undefined,
  )
}

export function engineDetailQueryOptions(driver: string) {
  const engineID = normalizeEngineID(driver)
  return queryOptions({
    queryKey: queryKeys.engine(engineID),
    queryFn: () => api.get<EngineView>(`/api/v1/engines/${engineID}`),
    // Static per build: an engine's reported capabilities never change at runtime.
    staleTime: Infinity,
  })
}

export function orgConnectionDirectoryQueryOptions(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId?: string,
) {
  return queryOptions({
    queryKey: connectionDirectoryQueryKey(slug, workspaceId, connectionId),
    queryFn: () =>
      api.get<DirectoryResponse>(
        `${schemaBase(slug, workspaceId, connectionId)}/directory`,
        schemaRequestOptions(sessionId),
      ),
    staleTime: 60_000,
    // Persistent snapshots are prepared asynchronously. Keep checking only
    // while the server reports that work is still in progress, then stop as
    // soon as a directory (or any terminal response) is available.
    refetchInterval: (query) => (query.state.data?.status === 'pending' ? 1_000 : false),
  })
}

export function orgConnectionSchemaSpecQueryOptions(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId?: string,
) {
  return queryOptions({
    queryKey: connectionSchemaSpecQueryKey(slug, workspaceId, connectionId),
    queryFn: () =>
      api.get<SchemaSpecResponse>(
        `${schemaBase(slug, workspaceId, connectionId)}/spec`,
        schemaRequestOptions(sessionId),
      ),
    staleTime: 5 * 60_000,
  })
}

export function connectionRelationshipsQueryKey(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  scope: ObjectRef['scope'],
) {
  return [
    'connection-relationships',
    slug,
    String(workspaceId),
    String(connectionId),
    JSON.stringify(scope),
  ] as const
}

export function connectionRelationshipsQueryKeyPrefix(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
) {
  return ['connection-relationships', slug, String(workspaceId), String(connectionId)] as const
}

export function orgConnectionRelationshipsQueryOptions(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId: string | undefined,
  scope: ObjectRef['scope'],
) {
  return queryOptions({
    queryKey: connectionRelationshipsQueryKey(slug, workspaceId, connectionId, scope),
    queryFn: async () => {
      const res = await api.get<RelationshipsResponse>(
        `${schemaBase(slug, workspaceId, connectionId)}/relationships?scope=${encodeURIComponent(JSON.stringify(scope))}`,
        schemaRequestOptions(sessionId),
      )
      return res.graph
    },
    staleTime: 3 * 60_000,
  })
}

export function connectionObjectsQueryKeyPrefix(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
) {
  return ['connection-object', slug, String(workspaceId), String(connectionId)] as const
}

export function connectionObjectQueryKey(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  ref: ObjectRef,
) {
  return [
    ...connectionObjectsQueryKeyPrefix(slug, workspaceId, connectionId),
    JSON.stringify(ref.scope),
    ref.kind,
    ref.name,
  ] as const
}

export function orgConnectionObjectQueryOptions(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId: string | undefined,
  ref: ObjectRef,
) {
  return queryOptions({
    queryKey: connectionObjectQueryKey(slug, workspaceId, connectionId, ref),
    queryFn: async () => {
      const res = await api.post<ObjectsResponse>(
        `${schemaBase(slug, workspaceId, connectionId)}/objects`,
        { refs: [ref] },
        schemaRequestOptions(sessionId),
      )
      return res.objects[0] ?? null
    },
    staleTime: 3 * 60_000,
  })
}

export function connectionGenerateStatementQueryKey(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  operation: StatementOperation,
  ref: ObjectRef,
) {
  return [
    'connection-generate-statement',
    slug,
    String(workspaceId),
    String(connectionId),
    operation,
    JSON.stringify(ref.scope),
    ref.kind,
    ref.name,
  ] as const
}

/** Requests a SQL statement template for one object. Persistent snapshots can
 *  generate without a live session, so sessionId is optional here — unlike
 *  applyConnectionSchemaEdit, which always mutates a live connection. */
export function orgConnectionGenerateStatementQueryOptions(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId: string | undefined,
  operation: StatementOperation,
  ref: ObjectRef,
) {
  return queryOptions({
    queryKey: connectionGenerateStatementQueryKey(slug, workspaceId, connectionId, operation, ref),
    queryFn: () =>
      api.post<GenerateStatementResponse>(
        `${schemaBase(slug, workspaceId, connectionId)}/statements`,
        { operation, ref },
        schemaRequestOptions(sessionId),
      ),
    retry: false,
  })
}

export function connectionPreviewQueryKey(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  ref: ObjectRef,
) {
  return [
    'connection-preview',
    slug,
    String(workspaceId),
    String(connectionId),
    JSON.stringify(ref.scope),
    ref.kind,
    ref.name,
  ] as const
}

export function connectionPreviewCountQueryKey(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  ref: ObjectRef,
) {
  return queryKeys.connectionPreviewCount(slug, String(workspaceId), String(connectionId), ref)
}

/** Runs a query on a connection. Pass useCursor to get a cursor-backed first
 *  page that can be paged with fetchConnectionCursorPage; pageSize sets that
 *  first page's size (the backend default is small). */
export type RunConnectionQueryOptions = {
  useCursor: boolean
  pageSize?: number
  /** Re-submits a statement the backend flagged as unsafe (e.g. missing
   *  WHERE) with explicit user confirmation to run it anyway. */
  confirmUnsafe?: boolean
  /** When set, the backend wraps `sql` in its EXPLAIN (or EXPLAIN ANALYZE)
   *  form for the connection's engine before executing it. The engine's
   *  EXPLAIN support is reported by GET /engines, not decided client-side. */
  explain?: 'plain' | 'analyze'
  signal?: AbortSignal
}

export function runConnectionQuery(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId: string,
  sql: string,
  options: RunConnectionQueryOptions,
) {
  return api.post<ResultSet>(
    `/api/v1/orgs/${slug}/workspaces/${workspaceId}/connections/${connectionId}/query`,
    {
      sql,
      use_cursor: options.useCursor,
      page_size: options.pageSize,
      confirm_unsafe: options.confirmUnsafe,
      explain: options.explain,
    },
    { headers: { 'X-Warden-Session': sessionId }, signal: options.signal },
  )
}

function transactionBase(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
) {
  return `/api/v1/orgs/${slug}/workspaces/${workspaceId}/connections/${connectionId}/transaction`
}

export function getConnectionTransactionStatus(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId: string,
) {
  return api.get<TransactionStatusResponse>(transactionBase(slug, workspaceId, connectionId), {
    headers: { 'X-Warden-Session': sessionId },
  })
}

export function setConnectionTransactionMode(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId: string,
  mode: TransactionMode,
) {
  return api.post<TransactionStatusResponse>(
    `${transactionBase(slug, workspaceId, connectionId)}/mode`,
    { mode },
    { headers: { 'X-Warden-Session': sessionId } },
  )
}

export function commitConnectionTransaction(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId: string,
) {
  return api.post<TransactionStatusResponse>(
    `${transactionBase(slug, workspaceId, connectionId)}/commit`,
    undefined,
    { headers: { 'X-Warden-Session': sessionId } },
  )
}

export function rollbackConnectionTransaction(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId: string,
) {
  return api.post<TransactionStatusResponse>(
    `${transactionBase(slug, workspaceId, connectionId)}/rollback`,
    undefined,
    { headers: { 'X-Warden-Session': sessionId } },
  )
}

/** Fetches the next page of an open query cursor (mirrors the result grid's
 *  paging; the cursor id authorizes the fetch, so no session header is sent). */
export function fetchConnectionCursorPage(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  cursorId: string,
  pageSize?: number,
  signal?: AbortSignal,
) {
  return api.post<ResultSet>(
    `/api/v1/orgs/${slug}/workspaces/${workspaceId}/connections/${connectionId}/query-cursors/${cursorId}/fetch`,
    { page_size: pageSize },
    { signal },
  )
}

export function closeConnectionQueryCursor(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  cursorId: string,
) {
  return api.delete<void>(
    `/api/v1/orgs/${slug}/workspaces/${workspaceId}/connections/${connectionId}/query-cursors/${cursorId}`,
  )
}

export function refreshConnectionSchema(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId?: string,
  ref?: ObjectRef,
) {
  return api.post<SchemaRefreshResponse>(
    `${schemaBase(slug, workspaceId, connectionId)}/refresh`,
    ref ? { ref } : undefined,
    schemaRequestOptions(sessionId),
  )
}

/** Applies a structured schema change. Requires a live session: the backend
 *  authorizes mutations against the session's connection and rejects the
 *  request without X-Warden-Session. */
export function applyConnectionSchemaEdit(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId: string,
  input: SchemaEditRequest,
) {
  return api.post<SchemaEditResponse>(
    `${schemaBase(slug, workspaceId, connectionId)}/mutations`,
    input,
    { headers: { 'X-Warden-Session': sessionId } },
  )
}

/**
 * Invalidates a connection's cached schema after a whole-connection refresh:
 * the directory and every lazily-fetched object detail. The server drops both on
 * refresh, so expanded object nodes must refetch — not just the directory.
 */
export function invalidateConnectionSchemaQueries(
  queryClient: QueryClient,
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
) {
  return Promise.all([
    queryClient.invalidateQueries({
      queryKey: connectionDirectoryQueryKey(slug, workspaceId, connectionId),
    }),
    queryClient.invalidateQueries({
      queryKey: connectionObjectsQueryKeyPrefix(slug, workspaceId, connectionId),
    }),
    queryClient.invalidateQueries({
      queryKey: connectionRelationshipsQueryKeyPrefix(slug, workspaceId, connectionId),
    }),
  ])
}
