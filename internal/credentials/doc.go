// Package credentials is the seam through which the execution runtime obtains
// target-database credentials. Providers resolve a ConnectionRef to
// Credentials; the runtime never reads encrypted columns or secret stores
// directly.
package credentials
