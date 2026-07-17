import { useQuery } from '@tanstack/react-query'
import { instanceEditionQueryOptions } from '#/lib/api/queries/instance'
import type { EnterpriseFeature } from '#/lib/enterprise/features'

export type FeatureState = 'loading' | 'error' | 'active' | 'locked' | 'unavailable'

export interface FeatureAccess {
  state: FeatureState
  retry: () => void
}

export function useEdition() {
  return useQuery(instanceEditionQueryOptions())
}

// Feature states are advisory only; the backend remains authoritative.
// Existing data wins over a transient refetch error so licensed UI does not
// disappear during a short network interruption.
export function useFeature(feature: EnterpriseFeature): FeatureAccess {
  const edition = useEdition()
  const retry = () => {
    void edition.refetch()
  }

  if (!edition.data) {
    return { state: edition.isError ? 'error' : 'loading', retry }
  }
  if (edition.data.licensed_features.includes(feature)) return { state: 'active', retry }
  if (edition.data.edition === 'enterprise') return { state: 'locked', retry }
  return { state: 'unavailable', retry }
}
