export type SortOrder = 'asc' | 'desc'

export interface Paginated<T> {
  items: T[]
  page: number
  page_size: number
  total: number
}

export interface Account {
  id: number
  email: string
  name: string
  is_active: boolean
  created_at?: string
  updated_at?: string
}

export interface Organization {
  id: number
  slug: string
  name: string
  schema_snapshots_enabled?: boolean
  mask_connection_credentials_on_edit?: boolean
  member_count?: number
  team_count?: number
  created_at: string
  updated_at: string
}

export interface AccountOrganization extends Organization {
  role: string
  member_count: number
  team_count: number
}

export interface Workspace {
  id: number
  org_id?: number
  owner_type: 'org' | 'space'
  owner_id: number
  name: string
  description?: string
  environment_count: number
  connection_count: number
  created_at: string
  updated_at: string
}

export type ResourceType = 'org' | 'workspace' | 'environment' | 'connection'
export type RoleScope = ResourceType

export interface PermissionDefinition {
  key: string
  label: string
  description: string
  group: string
}

export interface PermissionsCatalog {
  permissions: string[]
  permission_details: PermissionDefinition[]
  scope_map: Record<RoleScope, string[]>
  scope_details: Record<RoleScope, PermissionDefinition[]>
  resource_map: Record<ResourceType, string[]>
  resource_details: Record<ResourceType, PermissionDefinition[]>
}

export interface EffectivePermissions {
  resource_type: ResourceType
  resource_id: number
  permissions: string[]
}

export interface Environment {
  id: number
  workspace_id: number
  name: string
  description?: string
  created_at: string
  updated_at: string
}

export interface ScopeSegment {
  kind: string
  name: string
}

export type ScopePath = ScopeSegment[]

// ─── Query result types ─────────────────────────────────────────────────────

export type ColumnType =
  'text' | 'integer' | 'decimal' | 'boolean' | 'datetime' | 'json' | 'uuid' | 'bytes'

export interface ResultColumn {
  name: string
  type: ColumnType
  raw_type: string
  nullable: boolean
}

export type ValueType =
  'null' | 'text' | 'integer' | 'float' | 'decimal' | 'bool' | 'time' | 'bytes'

export interface ResultValue {
  type: ValueType
  text?: string
  integer?: number
  float?: number
  decimal?: string
  bool?: boolean
  time?: string
  bytes?: number[]
}

export type ResultRow = ResultValue[]

export type TransactionMode = 'auto' | 'manual'

export interface TransactionStatusResponse {
  mode: TransactionMode
  open: boolean
  pending_statements: number
  statements: string[]
}

export interface ResultSet {
  columns: ResultColumn[] | null
  rows: ResultRow[] | null
  rows_affected?: number
  duration_ms: number
  truncated: boolean
  rows_returned: number
  bytes_returned: number
  truncation_reason?: string
  query_cursor_id?: string
  exhausted?: boolean
  page_size?: number
  transaction: TransactionStatusResponse
}

/** One statement flagged by the backend's unsafe-DML safety check (e.g. an
 *  UPDATE/DELETE with no WHERE clause), returned as `error.details` on a
 *  `unsafe_query_confirmation_required` response. */
export interface UnsafeStatement {
  kind: string
  start_offset: number
  end_offset: number
}

export interface Connection {
  id: number
  workspace_id: number
  environment_id: number
  name: string
  driver: string
  access_mode: 'open' | 'restricted'
  default_scope?: ScopePath
  schema_snapshot_policy?: 'inherit' | 'disabled'
  created_at: string
  updated_at: string
}

export type QueryHistoryMode = 'backend' | 'local' | 'off'
export type QueryFavoritesMode = 'backend' | 'local' | 'off'

export interface QueryHistoryEntry {
  id: number
  connection_id: number
  account_id: number
  sql_text: string
  status: 'ok' | 'error' | 'cancelled'
  error_message: string | null
  duration_ms: number
  rows_affected: number
  executed_at: string
}

export interface QueryFavorite {
  id: number
  workspace_id: number
  account_id: number
  connection_id: number | null
  name: string
  sql_text: string
  created_at: string
  updated_at: string
}

export type WorkspaceFileObjectType = 'file' | 'folder'

export interface WorkspaceFile {
  id: number
  workspace_id: number
  parent_id?: number
  visibility: 'private' | 'shared'
  owner_account_id?: number
  object_type: WorkspaceFileObjectType
  name: string
  media_type?: string
  file_kind?: string
  current_content_id?: number
  created_by: number
  updated_by: number
  created_at: string
  updated_at: string
  content_hash?: string
  content_version?: number
  size_bytes?: number
}

export interface WorkspaceFilesResponse {
  files: WorkspaceFile[]
}

export interface WorkspaceFilePathSegment {
  id: number
  name: string
  object_type: WorkspaceFileObjectType
}

export interface WorkspaceFileBrowserResult {
  file: WorkspaceFile | null
  path: WorkspaceFilePathSegment[]
  children: WorkspaceFile[]
}

export interface WorkspaceFileSearchSnippet {
  line: number
  column: number
  excerpt: string
}

export interface WorkspaceFileSearchFileResult {
  file: WorkspaceFile
  path: WorkspaceFilePathSegment[]
  match_count: number
  snippets: WorkspaceFileSearchSnippet[]
}

export interface WorkspaceFileSearchResult {
  query: string
  results: WorkspaceFileSearchFileResult[]
  files_scanned: number
  truncated: boolean
}

export interface Team {
  id: number
  org_id: number
  slug: string
  name: string
  created_at: string
  updated_at: string
}

export interface TeamMember {
  team_id: number
  account_id: number
  email: string
  name: string
  created_at: string
}

export interface OrgMember {
  org_id: number
  account_id: number
  email: string
  name: string
  role: string
  joined_at: string
}

export interface OrganizationInvitation {
  id: string
  org_id: number
  email: string
  invited_by_account_id?: number
  expires_at: string
  delivery_status: 'sent' | 'failed' | 'disabled'
  last_delivery_attempt_at?: string
  created_at: string
  updated_at: string
  inviter_name?: string
  status?: 'pending' | 'expired'
}

export interface OrganizationInvitationMutationResponse {
  invitation: OrganizationInvitation
  invite_url: string
  delivery_status: OrganizationInvitation['delivery_status']
}

export interface OrganizationInvitationDetails {
  organization: Organization
  email: string
  expires_at: string
  status: 'pending' | 'expired'
  account_exists: boolean
  authenticated_as_invitee: boolean
}

export interface AcceptOrganizationInvitationResponse {
  organization: Organization
  access_token?: string
}

export interface WorkspaceMember {
  workspace_id: number
  account_id: number
  email: string
  name: string
  created_by?: number
  created_at: string
}

export interface WorkspaceMembershipSource {
  type: 'direct' | 'team'
  team_id?: number
  team_slug?: string
  team_name?: string
  created_at?: string
}

export interface WorkspaceEffectiveMember extends WorkspaceMember {
  is_direct_member: boolean
  membership_sources: WorkspaceMembershipSource[]
}

export interface WorkspaceTeam {
  workspace_id: number
  team_id: number
  slug: string
  name: string
  member_count: number
  created_by?: number
  created_at: string
}

export interface Role {
  id: number
  org_id: number
  workspace_id?: number
  name: string
  description?: string
  scope_type: RoleScope
  is_builtin: boolean
  created_at: string
  updated_at: string
  permissions?: string[]
}

export interface PolicyBinding {
  binding_kind: string
  binding_id: number
  subject_id: number
  subject_type: 'account' | 'team' | 'org_members' | 'workspace_members'
  subject_name: string
  resource_id: number
  resource_type: 'org' | 'workspace' | 'environment' | 'connection'
  resource_name: string
  role_id?: number
  role_name?: string
  created_at: string
}

export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

export interface InstanceSettings {
  instance_name: string
  instance_description: string
  support_email: string
  base_url: string
  personal_spaces_enabled: boolean
  jwt_access_token_ttl_seconds: number
  sessions_revocation_enabled: boolean
  query_max_result_rows: number
  query_max_result_bytes: number
  query_cursor_page_size: number
  exports_sync_max_bytes: number
  exports_background_max_bytes: number
  schema_snapshot_freshness_seconds: number
  file_revisions_enabled: boolean
  file_revisions_keep_latest: number
  query_history_mode: QueryHistoryMode
  query_history_retention_count: number
  query_history_retention_count_max: number
  query_favorites_mode: QueryFavoritesMode
  error_notification_email: string
  log_level: LogLevel
  database_query_tracing_enabled: boolean
  access_logs_enabled: boolean
  jobs_worker_count: number
  jobs_poll_interval_seconds: number
  jobs_claim_lease_seconds: number
  jobs_completed_retention_seconds: number
  smtp_enabled: boolean
  smtp_host: string
  smtp_port: number
  smtp_username: string
  /** Response-only: whether a secret is stored server-side. Never send this back in a PATCH. */
  smtp_password_configured: boolean
  smtp_from: string
}

/**
 * PATCH body for /api/v1/instance/settings. `smtp_password_configured` is response-only and
 * excluded; `smtp_password` is write-only: omit to preserve the saved secret, a string to
 * replace it, or null to clear it.
 */
export interface InstanceSettingsPatch extends Omit<InstanceSettings, 'smtp_password_configured'> {
  smtp_password?: string | null
}

export interface InstanceConfiguration {
  deployment_managed: boolean
  restart_required: boolean
  http_port: number
  deployment_mode: 'server' | 'desktop'
  access_mode: 'multi_user' | 'single_user'
  log_format: 'json' | 'text'
  database_driver: string
  database_automigrate: boolean
  tls_enabled: boolean
  file_storage_mode: 'file' | 'object'
  file_storage_backend: string
  sqlite_local_enabled: boolean
}

export interface OrganizationRuntimeOverrideValues {
  query_max_result_rows: number | null
  query_max_result_bytes: number | null
  exports_sync_max_bytes: number | null
  exports_background_max_bytes: number | null
  schema_snapshot_freshness_seconds: number | null
  file_revisions_enabled: boolean | null
  file_revisions_keep_latest: number | null
  query_history_mode: QueryHistoryMode | null
  query_history_retention_count: number | null
  query_favorites_mode: QueryFavoritesMode | null
}

export interface OrganizationRuntimeEffectiveValues {
  query_max_result_rows: number
  query_max_result_bytes: number
  exports_sync_max_bytes: number
  exports_background_max_bytes: number
  schema_snapshot_freshness_seconds: number
  file_revisions_enabled: boolean
  file_revisions_keep_latest: number
  query_history_mode: QueryHistoryMode
  query_history_retention_count: number
  query_favorites_mode: QueryFavoritesMode
}

export interface OrganizationRuntimeConstraints {
  query_max_result_rows_max: number
  query_max_result_bytes_max: number
  exports_sync_max_bytes_max: number
  exports_background_max_bytes_max: number
  schema_snapshot_freshness_seconds_min: number
  file_revisions_available: boolean
  file_revisions_keep_latest_max: number
  query_history_retention_count_max: number
}

export interface OrganizationRuntimeSettings {
  overrides: OrganizationRuntimeOverrideValues
  effective: OrganizationRuntimeEffectiveValues
  constraints: OrganizationRuntimeConstraints
  /** Present (and true) only on a PATCH response, when the mode just transitioned away from `backend` and rows still exist. */
  query_history_rows_remain?: boolean
  query_favorites_rows_remain?: boolean
}

export type OrganizationRuntimeSettingsPatch = {
  [K in keyof OrganizationRuntimeOverrideValues]?: OrganizationRuntimeOverrideValues[K]
}

export interface InstanceAdmin {
  account_id: number
  created_at: string
  account?: Account
}

export interface SessionResponse {
  account: Account
  organizations: Organization[]
  is_instance_admin: boolean
  personal_spaces_enabled: boolean
}

export interface SetupStatusResponse {
  configured: boolean
  access_mode: 'multi_user' | 'single_user'
}

export interface SetupResponse {
  account: Account
  access_token: string
  organization?: Organization
}

export interface AccessTokenResponse {
  access_token: string
}

export interface ListQuery {
  page?: number
  page_size?: number
  sort?: string
  order?: SortOrder
  q?: string
  [key: string]: string | number | boolean | undefined
}

export interface ObjectRef {
  scope: ScopePath
  kind: string
  name: string
}

export interface ScopeNode {
  path: ScopePath
  groups: ObjectGroup[]
  children?: ScopeNode[]
}

export interface ObjectGroup {
  kind: string
  objects: ObjectRef[]
}

export interface SchemaDirectory {
  connection: string
  engine: string
  default_scope: ScopePath
  generated_at: string
  roots: ScopeNode[]
}

export interface SchemaObjectKind {
  kind: string
  label: string
  plural_label: string
  order: number
  relational: boolean
  supports_diagram: boolean
  listing: 'enumerated' | 'searched'
}

export interface SchemaSpec {
  dialect: string
  kinds: SchemaObjectKind[]
}

export interface DbColumn {
  name: string
  data_type: string
  nullable: boolean
  default?: string
  ordinal: number
  attributes?: Record<string, unknown>
}

export interface DbForeignKey {
  name: string
  columns: string[]
  references: ObjectRef
  referenced_columns: string[]
}

export interface DbIndex {
  name: string
  columns: string[]
  unique: boolean
}

export interface RelationalDetail {
  columns: DbColumn[]
  primary_key?: string[]
  foreign_keys?: DbForeignKey[]
  indexes?: DbIndex[]
}

export interface ObjectField {
  name: string
  value: string
}

export interface ObjectRowSet {
  columns: string[]
  rows: string[][]
}

export interface ObjectSource {
  language: string
  body: string
}

export interface ObjectDescriptor {
  kind: 'fields' | 'rows' | 'source'
  title: string
  fields?: ObjectField[]
  rows?: ObjectRowSet
  source?: ObjectSource
}

export interface ObjectDetail {
  ref: ObjectRef
  relational?: RelationalDetail
  descriptors?: ObjectDescriptor[]
  attributes?: Record<string, unknown>
}

export interface DirectoryResponse {
  directory?: SchemaDirectory
  status?: 'pending'
  mode?: 'persistent' | 'ephemeral'
  job_id?: string
}

export interface SchemaSnapshotStatus {
  status: 'ready' | 'pending' | 'disabled'
  mode: 'persistent' | 'ephemeral'
  snapshot_id?: string
  generated_at?: string
  stale?: boolean
  job_id?: string
}

export interface SchemaRefreshResponse {
  status: 'ok'
  mode: 'persistent' | 'ephemeral'
  snapshot_id?: string
  generated_at?: string
}

export type StatementOperation = 'select' | 'insert' | 'update' | 'delete'

export interface StatementObjectSpec {
  kind: string
  operations: StatementOperation[]
}

/** Static, driver-advertised SQL-statement-generation capabilities, keyed by object kind. */
export interface StatementSpec {
  objects: StatementObjectSpec[]
}

export interface GenerateStatementRequest {
  operation: StatementOperation
  ref: ObjectRef
}

export interface GenerateStatementResponse {
  sql: string
}

export type SchemaEditOperation =
  'create_table' | 'drop_object' | 'drop_scope' | 'rename_column' | 'drop_column' | 'drop_index'

export interface SchemaEditColumn {
  name: string
  data_type: string
  nullable: boolean
  primary_key: boolean
}

/** Static, driver-advertised schema-editing capabilities. Never infer these in the UI. */
export interface SchemaEditSpec {
  operations: SchemaEditOperation[]
  column_types: string[]
  creatable_table_scope_kinds: string[]
  droppable_object_kinds: string[]
  droppable_scope_kinds: string[]
  supports_cascade: boolean
}

export interface SchemaEditRequest {
  operation: SchemaEditOperation
  scope?: ScopePath
  ref?: ObjectRef
  name?: string
  new_name?: string
  columns?: SchemaEditColumn[]
  cascade?: boolean
}

export interface SchemaEditStatus {
  status: 'available' | 'refresh_failed'
  mode: 'persistent' | 'ephemeral'
  snapshot_id?: string
  generated_at?: string
  stale?: boolean
}

export interface SchemaEditResponse {
  applied: boolean
  schema: SchemaEditStatus
  transaction: TransactionStatusResponse
}

export interface SchemaSpecResponse {
  spec: SchemaSpec
  editor?: SchemaEditSpec
  statements?: StatementSpec
}

export interface ObjectsResponse {
  objects: ObjectDetail[]
}

export interface Relationship {
  name: string
  source: ObjectRef
  columns: string[]
  references: ObjectRef
  referenced_columns: string[]
}

export interface RelationshipGraph {
  scope: ScopePath
  relationships: Relationship[] | null
}

export interface RelationshipsResponse {
  graph: RelationshipGraph
}

export type JobStatus = 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled'

export interface JobRecord {
  id: string
  type: string
  visibility: string
  status: JobStatus
  org_id?: number
  workspace_id?: number
  owner_account_id?: number
  run_at: string
  priority: number
  attempts: number
  max_attempts: number
  started_at?: string
  finished_at?: string
  cancel_requested_at?: string
  error_code?: string
  error_message?: string
  output?: unknown
  created_at: string
  updated_at: string
}

export interface JobEvent {
  id: string
  job_id: string
  level: 'info' | 'warn' | 'error'
  code: string
  message: string
  details?: unknown
  created_at: string
}

export interface JobEventPage {
  items: JobEvent[]
  next_after_id?: string
}

export interface ExportJobOutput {
  file_id: number
  filename: string
  format: string
  row_count: number
  byte_count: number
}
