import { apiRequest, parseAPIErrorPayload } from '#/lib/api/client'
import { getAccessToken } from '#/lib/auth/access-token'
import type { WorkspaceFile } from '#/lib/api/types'

export interface CreateFileInput {
  name: string
  object_type: 'file' | 'folder'
  parent_id: number | null
  media_type?: string
  file_kind?: string
}

export async function createPrivateWorkspaceFile(
  orgSlug: string,
  workspaceId: number,
  input: CreateFileInput,
): Promise<WorkspaceFile> {
  return apiRequest<WorkspaceFile>(
    `/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/files/private`,
    { method: 'POST', body: input },
  )
}

export async function getPrivateWorkspaceFileContent(
  orgSlug: string,
  workspaceId: number,
  fileId: number,
): Promise<{ text: string; etag: string }> {
  const url = `/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/files/private/${fileId}/content`
  const token = getAccessToken()
  const headers: Record<string, string> = {}
  if (token) headers['Authorization'] = `Bearer ${token}`

  const response = await fetch(url, { headers })
  if (!response.ok) {
    const err = new Error(`Failed to load file: ${response.statusText}`) as Error & { status: number }
    err.status = response.status
    throw err
  }
  const text = await response.text()
  const raw = response.headers.get('ETag') ?? ''
  return { text, etag: raw.replace(/^"|"$/g, '') }
}

export async function deletePrivateWorkspaceFile(
  orgSlug: string,
  workspaceId: number,
  fileId: number,
): Promise<void> {
  return apiRequest<void>(
    `/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/files/private/${fileId}`,
    { method: 'DELETE' },
  )
}

export async function updatePrivateWorkspaceFileContent(
  orgSlug: string,
  workspaceId: number,
  fileId: number,
  content: string,
  etag?: string,
): Promise<{ etag: string }> {
  const url = `/api/v1/orgs/${orgSlug}/workspaces/${workspaceId}/files/private/${fileId}/content`
  const token = getAccessToken()
  const headers: Record<string, string> = {
    'Content-Type': 'text/plain',
    'Accept': 'application/json',
  }
  if (token) headers['Authorization'] = `Bearer ${token}`
  if (etag) headers['If-Match'] = `"${etag}"`

  const response = await fetch(url, { method: 'PUT', headers, body: content })

  if (!response.ok) {
    const payload = await response.json().catch(() => null)
    const { message } = parseAPIErrorPayload(payload, response.statusText || 'Failed to save file')
    const err = new Error(message) as Error & { status: number }
    err.status = response.status
    throw err
  }

  const raw = response.headers.get('ETag') ?? ''
  const etag2 = raw.replace(/^"|"$/g, '')
  return { etag: etag2 }
}
