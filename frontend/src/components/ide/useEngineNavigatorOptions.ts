import { useQuery } from '@tanstack/react-query'
import { engineDetailQueryOptions } from '#/lib/api/queries/database'

export function useEngineNavigatorOptions(driver: string, enabled: boolean) {
  const { data } = useQuery({
    ...engineDetailQueryOptions(driver),
    enabled: enabled && driver !== '',
  })
  return {
    systemObjectsSupported: data?.supports_system_objects ?? false,
    showAllDatabasesSupported: data?.show_all_databases ?? false,
  }
}
