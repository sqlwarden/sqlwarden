export interface Tenant {
  id: string
  slug: string
  name: string
  created_at: string
  updated_at: string
}

export interface TenantMemberWithAccount {
  tenant_id: string
  account_id: string
  role: 'owner' | 'admin' | 'member'
  created_at: string
  account_name: string
  account_email: string
}

export interface OrgAuthInfo {
  has_sso: boolean
  sso_type: string | null
}
