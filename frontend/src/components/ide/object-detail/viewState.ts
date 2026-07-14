import { errorMessage } from '#/lib/api/errors'
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

/** Maps the detail query's status into a single view state. A missing session
 *  always wins — even over cached data — so a dead cloud session shows the
 *  reconnect pane instead of stale content with silently broken sub-queries.
 *  With a live session, cached data renders (`ready`) even while a fresh
 *  refetch is in flight; 403/410/501 collapse to the matching non-data state. */
export function resolveObjectViewState({
  hasSession,
  error,
  hasData,
}: ResolveInput): ObjectViewState {
  if (!hasSession) return { kind: 'no-session' }
  if (hasData) return { kind: 'ready' }
  if (error) {
    if (isApiError(error)) {
      if (error.status === 501) return { kind: 'unsupported' }
      if (error.status === 403) return { kind: 'forbidden' }
      if (error.status === 410) return { kind: 'no-session' }
    }
    return { kind: 'error', message: errorMessage(error, 'Failed to load object details.') }
  }
  return { kind: 'loading' }
}
