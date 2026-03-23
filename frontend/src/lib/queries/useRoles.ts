import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { orgsApi } from '#/lib/api/orgs'

export const roleKeys = {
  list: (slug: string) => ['org', slug, 'roles'] as const,
}

export function useRoles(slug: string) {
  return useQuery({ queryKey: roleKeys.list(slug), queryFn: () => orgsApi.listRoles(slug) })
}

export function useCreateRole(orgSlug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, description, actions }: { name: string; description: string; actions: string[] }) =>
      orgsApi.createRole(orgSlug, name, description, actions),
    onSuccess: () => qc.invalidateQueries({ queryKey: roleKeys.list(orgSlug) }),
  })
}

export function useDeleteRole(orgSlug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (roleId: string) => orgsApi.deleteRole(orgSlug, roleId),
    onSuccess: () => qc.invalidateQueries({ queryKey: roleKeys.list(orgSlug) }),
  })
}
