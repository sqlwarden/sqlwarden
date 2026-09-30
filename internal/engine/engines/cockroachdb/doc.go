// Package cockroachdb implements the CockroachDB engine per the extension
// pattern documented in postgres/doc.go: driver embeds postgres.Driver by
// value, and Go's method promotion satisfies engine.Driver and every optional
// capability interface through the embedded type automatically. Only the
// methods that genuinely diverge are overridden here:
//
//   - Dialect reports engine.DialectCockroachDB rather than
//     engine.DialectPostgres, since CockroachDB's SQL surface diverges enough
//     (different EXPLAIN grammar, catalog gaps) to need its own
//     dialect identity rather than reusing Postgres's unmodified.
//   - Tree (navigator.go) drops the folders CockroachDB does not support
//     (event triggers, extensions, foreign tables, aggregate functions,
//     partitions, rules) and marks systemSchemas (catalog.go) —
//     crdb_internal and pg_extension — as system schemas. CockroachDB exposes them as additional built-in virtual
//     schemas with no PostgreSQL equivalent, and their pg_catalog
//     compatibility rows are incomplete (e.g. NULL pg_get_function_arguments
//     for crdb_internal's builtins), so they are hidden with the other system
//     schemas unless the user asks for them.
//   - InspectObjects and InspectDefinition route function and procedure refs
//     to local functionObjects/functionDefinition (catalog.go) instead of
//     their postgres.Function* counterparts: CockroachDB's
//     pg_catalog.pg_language compatibility table is always empty, so
//     pg_proc.prolang never resolves against it and postgres's inner join on
//     pg_language silently returns zero rows for every function. The
//     language is instead recovered from the LANGUAGE clause
//     pg_get_functiondef always emits.
//   - Explain emits CockroachDB's own EXPLAIN grammar (no FORMAT TEXT option;
//     EXPLAIN ANALYZE is a top-level statement rather than an EXPLAIN option).
//
// Everything else — connection handling, TLS, SSH tunneling, DDL, parsing,
// classification, completion, and non-function object detail/definition
// inspection (RelationalObjects, SequenceObjects, TableDDL, ViewDefinition)
// — is inherited unmodified from postgres.Driver, since CockroachDB
// implements the PostgreSQL wire protocol and enough of
// pg_catalog/information_schema to satisfy those queries as-is.
package cockroachdb
