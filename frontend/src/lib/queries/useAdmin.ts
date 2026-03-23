import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { adminApi } from '#/lib/api/admin'

export function useAdminOrgs(page = 1) {
  return useQuery({ queryKey: ['admin', 'orgs', page], queryFn: () => adminApi.listOrgs(page) })
}
export function useAdminAccounts(page = 1) {
  return useQuery({ queryKey: ['admin', 'accounts', page], queryFn: () => adminApi.listAccounts(page) })
}
export function useInstanceSettings() {
  return useQuery({ queryKey: ['admin', 'settings'], queryFn: () => adminApi.getSettings() })
}
export function useCreateOrg() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ slug, name, ownerEmail }: { slug: string; name: string; ownerEmail: string }) =>
      adminApi.createOrg(slug, name, ownerEmail),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin', 'orgs'] }),
  })
}
export function useUpdateInstanceSetting() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) => adminApi.updateSetting(key, value),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['admin', 'settings'] }),
  })
}
