export interface Account {
  id: string
  email: string
  name: string
  is_active: boolean
  is_superadmin: boolean
  created_at: string
  updated_at: string
}

export interface LoginResponse {
  access_token: string
  expires_at: string
}
