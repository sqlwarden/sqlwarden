import {
  connectionSecretNames,
  type ConnectionSecretName,
  type ConnectionSecretViews,
  type ConnectionSecretsPayload,
} from '#/lib/api/types'

export type RestingSecretState = { kind: 'empty' } | { kind: 'saved'; revealable: boolean }

export type SecretFieldState =
  | RestingSecretState
  | { kind: 'managed'; source: string }
  | { kind: 'replaced'; value: string; previous: RestingSecretState }
  | { kind: 'cleared'; previous: RestingSecretState }

export type SecretAction = { type: 'edit'; value: string } | { type: 'clear' } | { type: 'restore' }

export type SecretStates = Record<ConnectionSecretName, SecretFieldState>

export function emptySecretStates(): SecretStates {
  return Object.fromEntries(
    connectionSecretNames.map((name) => [name, { kind: 'empty' }]),
  ) as SecretStates
}

export function secretStatesFromViews(views: ConnectionSecretViews | undefined): SecretStates {
  const states = emptySecretStates()
  for (const name of connectionSecretNames) {
    const view = views?.[name]
    if (!view?.set) continue
    states[name] =
      view.source && view.source !== 'stored'
        ? { kind: 'managed', source: view.source }
        : { kind: 'saved', revealable: view.revealable }
  }
  return states
}

function restingState(state: SecretFieldState): RestingSecretState | null {
  switch (state.kind) {
    case 'empty':
    case 'saved':
      return state
    case 'replaced':
    case 'cleared':
      return state.previous
    case 'managed':
      return null
  }
}

export function reduceSecret(state: SecretFieldState, action: SecretAction): SecretFieldState {
  const resting = restingState(state)
  if (!resting) return state

  switch (action.type) {
    case 'edit':
      if (state.kind === 'cleared') return state
      return action.value === ''
        ? resting
        : { kind: 'replaced', value: action.value, previous: resting }
    case 'clear':
      return resting.kind === 'saved' ? { kind: 'cleared', previous: resting } : resting
    case 'restore':
      return resting
  }
}

export function secretsPayload(states: SecretStates): ConnectionSecretsPayload {
  const payload: ConnectionSecretsPayload = {}
  for (const name of connectionSecretNames) {
    const state = states[name]
    if (state.kind === 'replaced') payload[name] = state.value
    else if (state.kind === 'cleared') payload[name] = null
  }
  return payload
}
