export interface Workspace {
  id: string
  tenant_id: string
  name: string
  description: string
  created_at: string
  updated_at: string
}

export interface Connection {
  id: string
  workspace_id: string
  name: string
  driver: 'postgres' | 'mysql' | 'sqlite'
  created_at: string
}

export interface TestConnectionResult {
  ok: boolean
  latency_ms: number
  error?: string
}
