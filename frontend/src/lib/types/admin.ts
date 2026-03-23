export interface InstanceSettings {
  auth_method: string
  personal_orgs_enabled: string
  sso_enforced: string
  [key: string]: string
}

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  page: number
  limit: number
}
