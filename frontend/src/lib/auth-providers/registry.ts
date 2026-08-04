import type { AuthProviderDefinition } from './types'

// No SSO providers are implemented yet — only credential-based login exists.
// Add an entry here (and the matching implementation) to light one up; the
// auth pages render this list generically and need no other changes.
export const authProviders: readonly AuthProviderDefinition[] = []
