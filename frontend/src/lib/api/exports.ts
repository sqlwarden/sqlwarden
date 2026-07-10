import { apiRequest, parseAPIErrorPayload } from '#/lib/api/client'
import { ApiError } from '#/lib/api/errors'
import { getAccessToken } from '#/lib/auth/access-token'
import type { JobEventPage, JobRecord } from '#/lib/api/types'

export interface ExportInput {
  sql: string
  format: string
  filename?: string
}

export async function createExport(
  orgSlug: string,
  workspaceId: number,
  connectionId: number,
  input: ExportInput,
): Promise<JobRecord> {
  return apiRequest<JobRecord>(
    `/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/connections/${connectionId}/exports`,
    { method: 'POST', body: input },
  )
}

/**
 * Raw fetch (not apiRequest) because the response body is a CSV stream, not
 * JSON. Must throw ApiError (not a plain Error, unlike files.ts's raw-fetch
 * helpers) so isSessionGone()/ensureSession's retry-on-410 logic recognizes
 * a dead session the same way it does for query requests.
 */
export async function downloadExport(
  orgSlug: string,
  workspaceId: number,
  connectionId: number,
  sessionId: string,
  input: ExportInput,
  signal?: AbortSignal,
): Promise<Response> {
  const token = getAccessToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    'X-Warden-Session': sessionId,
  }
  if (token) headers['Authorization'] = `Bearer ${token}`

  const response = await fetch(
    `/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/connections/${connectionId}/exports/download`,
    { method: 'POST', headers, body: JSON.stringify(input), signal },
  )
  if (!response.ok) {
    const payload = await response.json().catch(() => null)
    const parsed = parseAPIErrorPayload(payload, response.statusText || 'Export failed')
    throw new ApiError(parsed.message, response.status, { code: parsed.code, details: parsed.details, fieldErrors: parsed.fieldErrors })
  }
  return response
}

export async function cancelExportJob(orgSlug: string, workspaceId: number, jobId: string): Promise<JobRecord> {
  return apiRequest<JobRecord>(
    `/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/jobs/${jobId}/cancel`,
    { method: 'POST' },
  )
}

export async function getJobEvents(
  orgSlug: string,
  workspaceId: number,
  jobId: string,
  afterId?: string,
): Promise<JobEventPage> {
  return apiRequest<JobEventPage>(
    `/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/jobs/${jobId}/events`,
    { query: afterId ? { after_id: afterId } : undefined },
  )
}
