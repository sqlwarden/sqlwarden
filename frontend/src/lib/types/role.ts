export interface WorkspaceRoleWithActions {
  id: string
  tenant_id: string
  name: string
  description: string
  actions: string[]
}

export interface AccessGrant {
  id: string
  subject: string
  object: string
  action: string
  granted_by: string
  expires_at: string | null
  created_at: string
}
