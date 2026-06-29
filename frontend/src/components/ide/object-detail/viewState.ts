import { isApiError } from '#/lib/api/errors'

export type ObjectViewState =
  | { kind: 'no-session' }
  | { kind: 'loading' }
  | { kind: 'unsupported' }
  | { kind: 'forbidden' }
  | { kind: 'error'; message: string }
  | { kind: 'ready' }

export interface ResolveInput {
  hasSession: boolean
  isLoading: boolean
  error: unknown
  hasData: boolean
}

/** Maps the detail query's status into a single view state. Cached data renders
 *  (`ready`) even while a fresh authorized refetch is in flight; a missing
 *  session or a 403/410/501 collapses to the matching non-data state so cached
 *  bytes never show without a successful server authorization this session. */
export function resolveObjectViewState({ hasSession, error, hasData }: ResolveInput): ObjectViewState {
  if (hasData) return { kind: 'ready' }
  if (!hasSession) return { kind: 'no-session' }
  if (error) {
    if (isApiError(error)) {
      if (error.status === 501) return { kind: 'unsupported' }
      if (error.status === 403) return { kind: 'forbidden' }
      if (error.status === 410) return { kind: 'no-session' }
    }
    return { kind: 'error', message: error instanceof Error ? error.message : 'Failed to load object details.' }
  }
  return { kind: 'loading' }
}
