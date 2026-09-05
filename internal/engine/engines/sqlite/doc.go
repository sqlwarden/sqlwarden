// Package sqlite implements the SQLWarden engine for SQLite databases.
//
// Connectivity uses the pure-Go modernc.org/sqlite driver. SQL parsing,
// classification, safety checks, EXPLAIN validation, and autocomplete are
// backed by github.com/rqlite/sql, a pure-Go SQLite parser and AST — Omni,
// which backs the other engines, has no SQLite grammar.
//
// Supported baseline: SQLite 3.35+ (the RETURNING / STRICT / generated-column
// era). Older databases still connect and query; only the newest introspection
// details degrade gracefully.
//
// rqlite/sql's grammar is narrower than the SQLite runtime's: VACUUM, ATTACH,
// DETACH, data-modifying common table expressions, and a few other forms fail
// to parse. Such statements classify as Unknown, which the RBAC gate treats as
// requiring conn:execute — the safe direction. Safety checks fall back to the
// lexical heuristic for them.
//
// SQLite runs in-process against a local file, so there is no integration
// build tag: the full suite, including live-connection behavior, runs under
//
//	go test ./internal/engine/engines/sqlite/...
package sqlite
