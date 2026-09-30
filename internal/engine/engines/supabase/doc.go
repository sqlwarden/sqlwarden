// Package supabase implements the Supabase engine: vanilla PostgreSQL under
// the hood, with a fixed set of Supabase-managed schemas (auth, storage,
// realtime, ...) that carry the platform's own internals rather than
// user data. It embeds postgres.Driver by value and overrides only Tree, to
// mark those managed schemas as system schemas in the navigator. Every other capability — catalog inspection within a
// user schema, DDL generation, classification, completion, TLS/SSH tunnel
// support — is inherited unchanged via Go's method promotion.
package supabase
