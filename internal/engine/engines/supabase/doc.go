// Package supabase implements the Supabase engine: vanilla PostgreSQL under
// the hood, with a fixed set of Supabase-managed schemas (auth, storage,
// realtime, ...) that carry the platform's own internals rather than
// user data. It embeds postgres.Driver by value and overrides only
// InspectDirectory and DiscoverScopes, to keep those managed schemas out of
// the schema browser. Every other capability — catalog inspection within a
// user schema, DDL generation, classification, completion, TLS/SSH tunnel
// support — is inherited unchanged via Go's method promotion.
package supabase
