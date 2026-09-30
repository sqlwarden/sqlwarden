import { isApiError } from '#/lib/api/errors'
import type { NavigatorTree } from '#/lib/api/types'
import { diagramSupported } from './capability'

export type DiagramViewState =
  | 'missing-target'
  | 'no-session'
  | 'needs-load'
  | 'unsupported'
  | 'forbidden'
  | 'loading'
  | 'empty'
  | 'ready'

export function resolveDiagramViewState({
  hasTarget,
  hasConnection,
  hasSession,
  loadRequested,
  tree,
  treeError,
  listingError,
  relationshipsError,
  listingLoading,
  relationshipsLoading,
  presentCount,
}: {
  hasTarget: boolean
  hasConnection: boolean
  hasSession: boolean
  loadRequested: boolean
  tree?: NavigatorTree
  treeError: unknown
  listingError: unknown
  relationshipsError: unknown
  listingLoading: boolean
  relationshipsLoading: boolean
  presentCount: number
}): DiagramViewState {
  if (!hasTarget || !hasConnection) return 'missing-target'
  if (!hasSession) return 'no-session'
  if (!loadRequested) return 'needs-load'
  if (
    (isApiError(relationshipsError) && relationshipsError.status === 501) ||
    (tree != null && !diagramSupported(tree))
  )
    return 'unsupported'
  if (
    [treeError, listingError, relationshipsError].some(
      (error) => isApiError(error) && error.status === 403,
    )
  ) {
    return 'forbidden'
  }
  if (listingLoading || relationshipsLoading) return 'loading'
  if (presentCount === 0) return 'empty'
  return 'ready'
}
