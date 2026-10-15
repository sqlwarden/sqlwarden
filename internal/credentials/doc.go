// Package credentials owns access to target-database secret values. Providers
// resolve a scoped ConnectionRef, describe safe secret state, and reveal a
// single stored value only when an authorized caller explicitly requests it.
// Runtimes and transports never read encrypted columns or secret stores
// directly.
package credentials
