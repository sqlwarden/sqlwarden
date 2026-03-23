export interface Team {
  id: string
  tenant_id: string
  slug: string
  name: string
  created_at: string
}

export interface TeamMemberWithAccount {
  team_id: string
  account_id: string
  created_at: string
  account_name: string
  account_email: string
}
