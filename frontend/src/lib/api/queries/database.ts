import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import type {
  DirectoryResponse,
  EngineView,
  GenerateStatementResponse,
  ObjectDefinitionResponse,
  ObjectDescriptor,
  ObjectDetail,
  ObjectRef,
  ScopePath,
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
  namespace?: string
  qualifier?: string
  data_type?: string
}

export type SQLCompletionIndexObject = { schema: string; name: string; kind: string }

export type SQLCompletionIndexColumn = {
  schema: string
  table: string
  name: string
  type?: string
  nullable: boolean
}

export type SQLCompletionIndexResponse = {
  version: string
  default_schema: string
  schemas: string[]
  objects: SQLCompletionIndexObject[]
  columns: SQLCompletionIndexColumn[]
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
  context?: 'column' | 'relation' | 'keyword' | 'value' | 'any'
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

export function getConnectionCompletionIndex(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId: string | undefined,
  signal?: AbortSignal,
) {
  return api.get<SQLCompletionIndexResponse>(
    `${schemaBase(slug, workspaceId, connectionId)}/completion-index`,
    {
      ...(signal ? { signal } : {}),
      ...(sessionId ? { headers: { 'X-Warden-Session': sessionId } } : {}),
    },
  )
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
  scope?: ScopePath,
) {
  const key = connectionDirectoryQueryKey(slug, workspaceId, connectionId)
  const suffix = scope ? `?${new URLSearchParams({ scope: JSON.stringify(scope) })}` : ''
  // Keys on session presence so a session change never reuses another session's cached response.
  const sessionKeyPart = sessionId ?? 'no-session'
  return queryOptions({
    queryKey: scope ? [...key, JSON.stringify(scope), sessionKeyPart] : [...key, sessionKeyPart],
    queryFn: () =>
      api.get<DirectoryResponse>(
        `${schemaBase(slug, workspaceId, connectionId)}/directory${suffix}`,
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
    queryKey: [
      ...connectionSchemaSpecQueryKey(slug, workspaceId, connectionId),
      sessionId ?? 'no-session',
    ],
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
    queryFn: () =>
      api.get<RelationshipsResponse>(
        `${schemaBase(slug, workspaceId, connectionId)}/relationships?scope=${encodeURIComponent(JSON.stringify(scope))}`,
        schemaRequestOptions(sessionId),
      ),
    staleTime: 3 * 60_000,
    refetchInterval: (query) => (query.state.data?.status === 'pending' ? 1_000 : false),
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
  sessionId?: string,
) {
  // Keys on session presence so a session change never reuses another
  // session's cached response — notably a pending_connection result cached
  // while disconnected, which would otherwise sit stale for its full
  // staleTime with nothing to trigger a refetch until the next remount.
  const sessionKeyPart = sessionId ?? 'no-session'
  return [
    ...connectionObjectsQueryKeyPrefix(slug, workspaceId, connectionId),
    JSON.stringify(ref.scope),
    ref.kind,
    ref.name,
    sessionKeyPart,
  ] as const
}

export interface ObjectDetailResult {
  detail: ObjectDetail | null
  pendingConnection: boolean
  snapshotPending: boolean
}

export function orgConnectionObjectQueryOptions(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId: string | undefined,
  ref: ObjectRef,
) {
  return queryOptions({
    queryKey: connectionObjectQueryKey(slug, workspaceId, connectionId, ref, sessionId),
    queryFn: async (): Promise<ObjectDetailResult> => {
      const res = await api.post<ObjectsResponse>(
        `${schemaBase(slug, workspaceId, connectionId)}/objects`,
        { refs: [ref] },
        schemaRequestOptions(sessionId),
      )
      return {
        detail: res.objects?.[0] ?? null,
        pendingConnection: (res.pending_connection?.length ?? 0) > 0,
        snapshotPending: res.status === 'pending',
      }
    },
    staleTime: 3 * 60_000,
    refetchInterval: (query) => (query.state.data?.snapshotPending ? 1_000 : false),
  })
}

/** Identifies an object independent of the request that fetched it, so a
 *  single-ref refresh can find and invalidate its containing batch below. */
export function objectRefKey(ref: ObjectRef): string {
  return `${JSON.stringify(ref.scope)}:${ref.kind}:${ref.name}`
}

/** Query key for a batched object-detail fetch. `members` carries every ref's
 *  objectRefKey so a single-ref refresh can find and invalidate the batch it
 *  landed in without knowing how callers chunked their refs. */
export function connectionObjectsBatchQueryKey(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  refs: ObjectRef[],
) {
  const members = refs.map(objectRefKey).sort()
  return [
    ...connectionObjectsQueryKeyPrefix(slug, workspaceId, connectionId),
    'batch',
    members,
  ] as const
}

/** Fetches detail for many refs in one request. The backend already batches
 *  driver inspection internally, so this trades N single-ref round-trips for
 *  one call per chunk the caller passes in. */
export function orgConnectionObjectsQueryOptions(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId: string | undefined,
  refs: ObjectRef[],
) {
  return queryOptions({
    queryKey: connectionObjectsBatchQueryKey(slug, workspaceId, connectionId, refs),
    queryFn: () =>
      api.post<ObjectsResponse>(
        `${schemaBase(slug, workspaceId, connectionId)}/objects`,
        { refs },
        schemaRequestOptions(sessionId),
      ),
    staleTime: 3 * 60_000,
    refetchInterval: (query) => (query.state.data?.status === 'pending' ? 1_000 : false),
  })
}

/** Matches any cached batch query (from orgConnectionObjectsQueryOptions) that
 *  contains `ref`, regardless of how it was chunked. Pass to
 *  queryClient.invalidateQueries alongside connectionObjectQueryKey so a
 *  single-object refresh reaches both the single-ref cache entry and any
 *  diagram/tree batch it was fetched as part of. */
export function connectionObjectsBatchContainingPredicate(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  ref: ObjectRef,
) {
  const prefix = connectionObjectsQueryKeyPrefix(slug, workspaceId, connectionId)
  const target = objectRefKey(ref)
  return (query: { queryKey: readonly unknown[] }) => {
    const key = query.queryKey
    if (key.length !== prefix.length + 2) return false
    for (let i = 0; i < prefix.length; i++) if (key[i] !== prefix[i]) return false
    if (key[prefix.length] !== 'batch') return false
    const members = key[prefix.length + 1]
    return Array.isArray(members) && members.includes(target)
  }
}

export function connectionObjectDefinitionQueryKeyPrefix(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
) {
  return ['connection-object-definition', slug, String(workspaceId), String(connectionId)] as const
}

export function connectionObjectDefinitionQueryKey(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  ref: ObjectRef,
) {
  return [
    ...connectionObjectDefinitionQueryKeyPrefix(slug, workspaceId, connectionId),
    JSON.stringify(ref.scope),
    ref.kind,
    ref.name,
  ] as const
}

/** Fetches one object's canonical text definition on demand. Engines that omit
 *  the definition from bulk object inspection for cost reasons (Oracle) serve it
 *  here; the object-detail DDL view falls back to this when no inline source
 *  descriptor is present. Persistent snapshots resolve it without a live session,
 *  so sessionId is optional. */
export function orgConnectionObjectDefinitionQueryOptions(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId: string | undefined,
  ref: ObjectRef,
  enabled: boolean,
) {
  const params = new URLSearchParams({
    scope: JSON.stringify(ref.scope),
    kind: ref.kind,
    name: ref.name,
  })
  return queryOptions({
    queryKey: connectionObjectDefinitionQueryKey(slug, workspaceId, connectionId, ref),
    queryFn: async (): Promise<ObjectDescriptor | null> => {
      const res = await api.get<ObjectDefinitionResponse>(
        `${schemaBase(slug, workspaceId, connectionId)}/object/definition?${params.toString()}`,
        schemaRequestOptions(sessionId),
      )
      return res.descriptor
    },
    enabled,
    staleTime: 5 * 60_000,
    retry: false,
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

export function loadSchemaScope(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  scope: ScopePath,
  sessionId?: string,
) {
  return api.post<SchemaRefreshResponse>(
    `${schemaBase(slug, workspaceId, connectionId)}/scope/load`,
    { scope },
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
 * refresh, so expanded object nodes must refetch — not just the directory. This
 * includes the standalone object-definition query the DDL view falls back to
 * (engines that omit source text from bulk object inspection, e.g. Oracle, and
 * any non-relational kind served through it) — without it, the DDL tab keeps
 * showing pre-refresh text until a full page reload clears the whole cache.
 */
/** Caps how many previously-expanded object rows refetch at once on
 *  reconnect. Each row is its own request (see orgConnectionObjectQueryOptions),
 *  so refetching every active one in parallel would fire a request burst
 *  proportional to how much of the tree the user had open. */
const RECONNECT_OBJECT_REFETCH_CONCURRENCY = 4

export async function invalidateConnectionSchemaQueries(
  queryClient: QueryClient,
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
) {
  const objectsPrefix = connectionObjectsQueryKeyPrefix(slug, workspaceId, connectionId)
  await Promise.all([
    queryClient.invalidateQueries({
      queryKey: connectionDirectoryQueryKey(slug, workspaceId, connectionId),
    }),
    queryClient.invalidateQueries({ queryKey: objectsPrefix, refetchType: 'none' }),
    queryClient.invalidateQueries({
      queryKey: connectionObjectDefinitionQueryKeyPrefix(slug, workspaceId, connectionId),
    }),
    queryClient.invalidateQueries({
      queryKey: connectionRelationshipsQueryKeyPrefix(slug, workspaceId, connectionId),
    }),
  ])
  await refetchThrottled(queryClient, objectsPrefix, RECONNECT_OBJECT_REFETCH_CONCURRENCY)
}

async function refetchThrottled(
  queryClient: QueryClient,
  queryKeyPrefix: readonly unknown[],
  concurrency: number,
) {
  const queries = queryClient.getQueryCache().findAll({ queryKey: queryKeyPrefix, type: 'active' })
  for (let i = 0; i < queries.length; i += concurrency) {
    const batch = queries.slice(i, i + concurrency)
    await Promise.all(
      batch.map((query) => queryClient.refetchQueries({ queryKey: query.queryKey, exact: true })),
    )
  }
}
