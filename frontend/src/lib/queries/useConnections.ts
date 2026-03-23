import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { connectionsApi } from '#/lib/api/connections'

export const connKeys = {
  list: (orgSlug: string, wsId: string) => ['org', orgSlug, 'workspaces', wsId, 'connections'] as const,
}
export function useConnections(orgSlug: string, wsId: string) {
  return useQuery({ queryKey: connKeys.list(orgSlug, wsId), queryFn: () => connectionsApi.list(orgSlug, wsId) })
}
export function useCreateConnection(orgSlug: string, wsId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, driver, dsn }: { name: string; driver: string; dsn: string }) =>
      connectionsApi.create(orgSlug, wsId, name, driver, dsn),
    onSuccess: () => qc.invalidateQueries({ queryKey: connKeys.list(orgSlug, wsId) }),
  })
}
export function useDeleteConnection(orgSlug: string, wsId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (connId: string) => connectionsApi.delete(orgSlug, wsId, connId),
    onSuccess: () => qc.invalidateQueries({ queryKey: connKeys.list(orgSlug, wsId) }),
  })
}
