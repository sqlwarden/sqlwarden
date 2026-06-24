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

// ─── Query result types ─────────────────────────────────────────────────────

export type ColumnType = 'text' | 'integer' | 'decimal' | 'boolean' | 'datetime' | 'json' | 'uuid' | 'bytes'

export interface ResultColumn {
  name: string
  type: ColumnType
  raw_type: string
  nullable: boolean
}

export type ValueType = 'null' | 'text' | 'integer' | 'float' | 'decimal' | 'bool' | 'time' | 'bytes'

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

export interface ResultSet {
  columns: ResultColumn[] | null
  rows: ResultRow[] | null
  duration_ms: number
  truncated: boolean
  rows_returned: number
  bytes_returned: number
  truncation_reason?: string
}

export interface Connection {
  id: number
  workspace_id: number
  environment_id: number
  name: string
  driver: string
  access_mode: 'open' | 'restricted'
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

export interface InstanceSettings {
  instance_name: string
  instance_description: string
  support_email: string
  public_url: string
  personal_spaces_enabled: boolean
  deployment_mode: 'server' | 'desktop'
  access_mode: 'multi_user' | 'single_user'
  single_user_mode: boolean
  personal_spaces_default: boolean
  runtime_settings_readonly: boolean
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
  namespace: string
  kind: string
  name: string
}

export interface CatalogObjectGroup {
  kind: string
  objects: ObjectRef[]
}

export interface CatalogNamespace {
  name: string
  groups: CatalogObjectGroup[] | null
}

export interface SchemaCatalog {
  connection: string
  dialect: string
  database: string
  generated_at: string
  namespaces: CatalogNamespace[] | null
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

export interface CatalogResponse {
  catalog: SchemaCatalog
}

export interface SchemaSpecResponse {
  spec: SchemaSpec
}

export interface ObjectsResponse {
  objects: ObjectDetail[]
}
