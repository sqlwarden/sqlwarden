import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { workspacesApi } from '#/lib/api/workspaces'

export const wsKeys = {
  list: (orgSlug: string) => ['org', orgSlug, 'workspaces'] as const,
  detail: (orgSlug: string, wsId: string) => ['org', orgSlug, 'workspaces', wsId] as const,
  access: (orgSlug: string, wsId: string) => ['org', orgSlug, 'workspaces', wsId, 'access'] as const,
}
export function useWorkspaces(orgSlug: string) {
  return useQuery({ queryKey: wsKeys.list(orgSlug), queryFn: () => workspacesApi.list(orgSlug) })
}
export function useWorkspace(orgSlug: string, wsId: string) {
  return useQuery({ queryKey: wsKeys.detail(orgSlug, wsId), queryFn: () => workspacesApi.get(orgSlug, wsId) })
}
export function useAccessGrants(orgSlug: string, wsId: string) {
  return useQuery({ queryKey: wsKeys.access(orgSlug, wsId), queryFn: () => workspacesApi.listAccess(orgSlug, wsId) })
}
export function useCreateWorkspace(orgSlug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, description }: { name: string; description: string }) => workspacesApi.create(orgSlug, name, description),
    onSuccess: () => qc.invalidateQueries({ queryKey: wsKeys.list(orgSlug) }),
  })
}
export function useGrantAccess(orgSlug: string, wsId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ subject, action, expiresAt }: { subject: string; action: string; expiresAt?: string }) =>
      workspacesApi.grantAccess(orgSlug, wsId, subject, action, expiresAt),
    onSuccess: () => qc.invalidateQueries({ queryKey: wsKeys.access(orgSlug, wsId) }),
  })
}
export function useRevokeAccess(orgSlug: string, wsId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (subject: string) => workspacesApi.revokeAccess(orgSlug, wsId, subject),
    onSuccess: () => qc.invalidateQueries({ queryKey: wsKeys.access(orgSlug, wsId) }),
  })
}
