import { useQuery } from '@tanstack/react-query'
import { instanceCapabilitiesQueryOptions } from '#/lib/api/queries/instance'

export type CapabilityState = 'loading' | 'error' | 'active' | 'locked'

export interface CapabilityAccess {
  state: CapabilityState
  retry: () => void
}

export function useCapabilities() {
  return useQuery(instanceCapabilitiesQueryOptions())
}

// Compiled extension implementations use this generic availability state.
// The extension decides whether a locked state represents licensing,
// configuration, or another deployment constraint.
export function useCapability(capability: string): CapabilityAccess {
  const state = useCapabilities()
  const retry = () => {
    void state.refetch()
  }

  if (!state.data) {
    return { state: state.isError ? 'error' : 'loading', retry }
  }
  if (state.data.capabilities.includes(capability)) return { state: 'active', retry }
  return { state: 'locked', retry }
}
