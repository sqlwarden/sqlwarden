import { useQuery } from '@tanstack/react-query'
import { instanceEditionQueryOptions } from '#/lib/api/queries/instance'

export type FeatureState = 'active' | 'locked' | 'unavailable'

export function useEdition() {
  return useQuery(instanceEditionQueryOptions())
}

// Feature states: 'active' (licensed on this server), 'locked' (enterprise
// binary without a license for this feature), 'unavailable' (community
// binary). Advisory only — the backend enforces licensing server-side.
export function useFeature(feature: string): FeatureState {
  const edition = useEdition()
  if (edition.data?.licensed_features.includes(feature)) return 'active'
  if (edition.data?.edition === 'enterprise') return 'locked'
  return 'unavailable'
}
