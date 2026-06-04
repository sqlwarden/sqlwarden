export const AUTH_INVALIDATED_EVENT = 'sqlwarden:auth-invalidated'

export function notifyAuthInvalidated() {
  window.dispatchEvent(new CustomEvent(AUTH_INVALIDATED_EVENT))
}
