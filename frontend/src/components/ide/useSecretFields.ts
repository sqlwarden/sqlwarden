import { useCallback, useMemo, useState } from 'react'
import type { ConnectionSecretName, ConnectionSecretViews } from '#/lib/api/types'
import {
  emptySecretStates,
  reduceSecret,
  secretStatesFromViews,
  secretsPayload,
  type SecretAction,
  type SecretFieldState,
  type SecretStates,
} from './secretField'

export interface SecretBinding {
  state: SecretFieldState
  dispatch: (action: SecretAction) => void
  reveal?: () => Promise<string>
}

export type BindSecret = (name: ConnectionSecretName) => SecretBinding

export function useSecretFields({
  reveal,
}: {
  reveal?: (name: ConnectionSecretName) => Promise<string>
} = {}) {
  const [states, setStates] = useState<SecretStates>(emptySecretStates)

  const dispatch = useCallback((name: ConnectionSecretName, action: SecretAction) => {
    setStates((current) => ({ ...current, [name]: reduceSecret(current[name], action) }))
  }, [])

  const load = useCallback((views: ConnectionSecretViews | undefined) => {
    setStates(secretStatesFromViews(views))
  }, [])

  const bind: BindSecret = useCallback(
    (name) => ({
      state: states[name],
      dispatch: (action) => dispatch(name, action),
      reveal: reveal ? () => reveal(name) : undefined,
    }),
    [states, dispatch, reveal],
  )

  const payload = useMemo(() => secretsPayload(states), [states])

  return { states, bind, load, payload }
}
