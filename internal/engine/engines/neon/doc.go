// Package neon implements the Neon engine: serverless Postgres with an
// identical wire protocol and catalog to plain PostgreSQL. It embeds
// postgres.Driver by value and overrides only Connect, to tolerate the
// connection latency a suspended (autosuspended) compute incurs on its first
// query after idling. Every other capability — catalog inspection, DDL
// generation, classification, completion, TLS/SSH tunnel support — is
// inherited unchanged via Go's method promotion.
package neon
