import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { api } from '#/lib/api/client'
import type {
  CatalogResponse,
  ObjectRef,
  ObjectsResponse,
  RelationshipsResponse,
  ResultSet,
  SchemaSpecResponse,
} from '#/lib/api/types'
import { queryKeys } from '#/lib/api/query-keys'

export function connectionCatalogQueryKey(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
) {
  return ['connection-catalog', slug, String(workspaceId), String(connectionId)] as const
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

export function getSQLCompletionVocabulary(driver: string, signal?: AbortSignal) {
  const normalized =
    driver === 'postgresql'
      ? 'postgres'
      : driver === 'mariadb'
        ? 'mysql'
        : driver === 'sqlite3'
          ? 'sqlite'
          : driver
  return api.get<SQLCompletionVocabulary>(
    `/api/v1/engines/${normalized}/completion-vocabulary`,
    signal ? { signal } : undefined,
  )
}

export function orgConnectionCatalogQueryOptions(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId?: string,
) {
  return queryOptions({
    queryKey: connectionCatalogQueryKey(slug, workspaceId, connectionId),
    queryFn: () =>
      api.get<CatalogResponse>(
        `${schemaBase(slug, workspaceId, connectionId)}/catalog`,
        schemaRequestOptions(sessionId),
      ),
    staleTime: 60_000,
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
  namespace: string,
) {
  return [
    'connection-relationships',
    slug,
    String(workspaceId),
    String(connectionId),
    namespace,
  ] as const
}

export function orgConnectionRelationshipsQueryOptions(
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
  sessionId: string | undefined,
  namespace: string,
) {
  return queryOptions({
    queryKey: connectionRelationshipsQueryKey(slug, workspaceId, connectionId, namespace),
    queryFn: async () => {
      const res = await api.get<RelationshipsResponse>(
        `${schemaBase(slug, workspaceId, connectionId)}/relationships?namespace=${encodeURIComponent(namespace)}`,
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
    ref.namespace,
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
    ref.namespace,
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
    { sql, use_cursor: options.useCursor, page_size: options.pageSize },
    { headers: { 'X-Warden-Session': sessionId }, signal: options.signal },
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
  return api.post<{ status: string }>(
    `${schemaBase(slug, workspaceId, connectionId)}/refresh`,
    ref ? { ref } : undefined,
    schemaRequestOptions(sessionId),
  )
}

/**
 * Invalidates a connection's cached schema after a whole-connection refresh:
 * the catalog and every lazily-fetched object detail. The server drops both on
 * refresh, so expanded object nodes must refetch — not just the catalog.
 */
export function invalidateConnectionSchemaQueries(
  queryClient: QueryClient,
  slug: string,
  workspaceId: string | number,
  connectionId: string | number,
) {
  return Promise.all([
    queryClient.invalidateQueries({
      queryKey: connectionCatalogQueryKey(slug, workspaceId, connectionId),
    }),
    queryClient.invalidateQueries({
      queryKey: connectionObjectsQueryKeyPrefix(slug, workspaceId, connectionId),
    }),
  ])
}
