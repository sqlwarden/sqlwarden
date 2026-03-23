import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { orgsApi } from '#/lib/api/orgs'

export const teamKeys = {
  list: (slug: string) => ['org', slug, 'teams'] as const,
  members: (slug: string, teamSlug: string) => ['org', slug, 'teams', teamSlug, 'members'] as const,
}
export function useTeams(slug: string) {
  return useQuery({ queryKey: teamKeys.list(slug), queryFn: () => orgsApi.listTeams(slug) })
}
export function useTeamMembers(slug: string, teamSlug: string) {
  return useQuery({ queryKey: teamKeys.members(slug, teamSlug), queryFn: () => orgsApi.listTeamMembers(slug, teamSlug), enabled: !!teamSlug })
}
export function useCreateTeam(orgSlug: string) {
  const qc = useQueryClient()
  return useMutation({ mutationFn: ({ slug, name }: { slug: string; name: string }) => orgsApi.createTeam(orgSlug, slug, name), onSuccess: () => qc.invalidateQueries({ queryKey: teamKeys.list(orgSlug) }) })
}
export function useDeleteTeam(orgSlug: string) {
  const qc = useQueryClient()
  return useMutation({ mutationFn: (teamSlug: string) => orgsApi.deleteTeam(orgSlug, teamSlug), onSuccess: () => qc.invalidateQueries({ queryKey: teamKeys.list(orgSlug) }) })
}
export function useAddTeamMember(orgSlug: string, teamSlug: string) {
  const qc = useQueryClient()
  return useMutation({ mutationFn: (accountId: string) => orgsApi.addTeamMember(orgSlug, teamSlug, accountId), onSuccess: () => qc.invalidateQueries({ queryKey: teamKeys.members(orgSlug, teamSlug) }) })
}
export function useRemoveTeamMember(orgSlug: string, teamSlug: string) {
  const qc = useQueryClient()
  return useMutation({ mutationFn: (accountId: string) => orgsApi.removeTeamMember(orgSlug, teamSlug, accountId), onSuccess: () => qc.invalidateQueries({ queryKey: teamKeys.members(orgSlug, teamSlug) }) })
}
