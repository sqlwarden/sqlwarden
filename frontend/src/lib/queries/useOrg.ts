import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { orgsApi } from '#/lib/api/orgs'

export const orgKeys = {
  detail: (slug: string) => ['org', slug] as const,
  members: (slug: string) => ['org', slug, 'members'] as const,
}

export function useOrg(slug: string) {
  return useQuery({ queryKey: orgKeys.detail(slug), queryFn: () => orgsApi.getOrg(slug) })
}

export function useOrgMembers(slug: string) {
  return useQuery({ queryKey: orgKeys.members(slug), queryFn: () => orgsApi.listMembers(slug) })
}

export function useInviteMember(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ email, role }: { email: string; role: string }) => orgsApi.inviteMember(slug, email, role),
    onSuccess: () => qc.invalidateQueries({ queryKey: orgKeys.members(slug) }),
  })
}

export function useChangeMemberRole(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ accountId, role }: { accountId: string; role: string }) =>
      orgsApi.updateMemberRole(slug, accountId, role),
    onSuccess: () => qc.invalidateQueries({ queryKey: orgKeys.members(slug) }),
  })
}

export function useRemoveMember(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (accountId: string) => orgsApi.removeMember(slug, accountId),
    onSuccess: () => qc.invalidateQueries({ queryKey: orgKeys.members(slug) }),
  })
}

export function useUpdateOrg(slug: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => orgsApi.updateOrg(slug, name),
    onSuccess: () => qc.invalidateQueries({ queryKey: orgKeys.detail(slug) }),
  })
}
