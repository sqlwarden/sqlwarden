// Package cockroachdb implements the CockroachDB engine per the extension
// pattern documented in postgres/doc.go: driver embeds postgres.Driver by
// value, and Go's method promotion satisfies engine.Driver and every optional
// capability interface through the embedded type automatically. Only the
// methods that genuinely diverge are overridden here:
//
//   - Dialect reports engine.DialectCockroachDB rather than
//     engine.DialectPostgres, since CockroachDB's SQL surface diverges enough
//     (no materialized views, different EXPLAIN grammar) to need its own
//     dialect identity rather than reusing Postgres's unmodified.
//   - SchemaSpec/InspectDirectory/InspectObjects drop the materialized_view
//     kind: CockroachDB has no CREATE MATERIALIZED VIEW support.
//   - InspectDirectory's row-count step uses a local attachRowCounts (catalog.go)
//     instead of postgres.AttachRowCounts: CockroachDB's pg_class compatibility
//     view reports pg_class.reltuples as NULL until a table has been scanned by
//     its stats collector, which postgres.AttachRowCounts's non-nullable scan
//     does not expect.
//   - InspectDirectory and DiscoverScopes filter out systemSchemas
//     (catalog.go) — crdb_internal and pg_extension. InspectDirectory still
//     calls the shared postgres.Catalog* functions unmodified, but wraps each
//     callback in a Go-side check rather than re-deriving their SQL, and
//     DiscoverScopes delegates to postgres.Driver.DiscoverScopes and filters
//     the resulting scopes. CockroachDB exposes crdb_internal and
//     pg_extension as additional
//     built-in virtual schemas with no PostgreSQL equivalent, and their
//     pg_catalog compatibility rows are incomplete (e.g. NULL
//     pg_get_function_arguments for crdb_internal's builtins), so they must
//     be excluded from user-facing catalog listings and scope discovery the
//     same way pg_catalog and information_schema already are.
//   - InspectObjects' function branch and InspectDefinition's function case
//     use local functionObjects/functionDefinition (catalog.go) instead of
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
// pg_catalog/information_schema to satisfy those queries as-is once system
// schemas are filtered out at enumeration time.
package cockroachdb
