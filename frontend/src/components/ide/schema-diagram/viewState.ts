import { isApiError } from '#/lib/api/errors'
import type { SchemaSpec } from '#/lib/api/types'

export type DiagramViewState =
  'missing-target' | 'no-session' | 'unsupported' | 'forbidden' | 'loading' | 'empty' | 'ready'

export function resolveDiagramViewState({
  hasTarget,
  hasConnection,
  hasSession,
  spec,
  specError,
  directoryError,
  relationshipsError,
  directoryLoading,
  relationshipsLoading,
  presentCount,
}: {
  hasTarget: boolean
  hasConnection: boolean
  hasSession: boolean
  spec?: SchemaSpec
  specError: unknown
  directoryError: unknown
  relationshipsError: unknown
  directoryLoading: boolean
  relationshipsLoading: boolean
  presentCount: number
}): DiagramViewState {
  if (!hasTarget || !hasConnection) return 'missing-target'
  if (!hasSession) return 'no-session'
  if (
    (isApiError(relationshipsError) && relationshipsError.status === 501) ||
    (spec != null && !spec.kinds.some((kind) => kind.supports_diagram))
  )
    return 'unsupported'
  if (
    [specError, directoryError, relationshipsError].some(
      (error) => isApiError(error) && error.status === 403,
    )
  ) {
    return 'forbidden'
  }
  if (directoryLoading || relationshipsLoading) return 'loading'
  if (presentCount === 0) return 'empty'
  return 'ready'
}
