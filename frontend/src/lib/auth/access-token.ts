const ACCESS_TOKEN_KEY = 'sqlwarden.access_token'
let storage: 'persistent' | 'memory' = 'persistent'
let memoryToken: string | null = null

export function configureInMemoryAccessTokens() {
  storage = 'memory'
  // Remove credentials written by desktop builds predating SQLW-101.
  window.localStorage.removeItem(ACCESS_TOKEN_KEY)
}

export function configurePersistentAccessTokens() {
  storage = 'persistent'
  memoryToken = null
}

export function getAccessToken() {
  if (storage === 'memory') return memoryToken
  return window.localStorage.getItem(ACCESS_TOKEN_KEY)
}

export function setAccessToken(token: string) {
  if (storage === 'memory') {
    memoryToken = token
    return
  }
  window.localStorage.setItem(ACCESS_TOKEN_KEY, token)
}

export function clearAccessToken() {
  memoryToken = null
  window.localStorage.removeItem(ACCESS_TOKEN_KEY)
}
