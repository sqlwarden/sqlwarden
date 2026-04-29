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
  subject_type: 'account' | 'team' | 'org_members'
  subject_name: string
  resource_id: number
  resource_type: 'org' | 'workspace' | 'environment' | 'connection'
  resource_name: string
  role_id?: number
  role_name?: string
  created_at: string
}

export interface InstanceSettings {
  personal_spaces_enabled: boolean
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
}

export interface SetupResponse {
  account: Account
  access_token: string
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
