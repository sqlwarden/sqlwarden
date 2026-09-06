// Package yugabyte implements the YugabyteDB engine per the extension
// pattern documented in postgres/doc.go: driver embeds postgres.Driver by
// value, and Go's method promotion satisfies engine.Driver and every
// optional capability interface through the embedded type automatically.
//
// Unlike the other compatible engines in this repository, YugabyteDB's YSQL
// API is not a reimplementation of the PostgreSQL wire protocol but a fork
// of the PostgreSQL server source itself, so it needs no catalog or grammar
// overrides: pg_catalog/information_schema shape, EXPLAIN output, DDL
// syntax, and object definitions all match what postgres.Driver already
// produces. The only override is Dialect, which reports
// engine.DialectPostgres explicitly rather than leaving it to the embedded
// default, matching the identity this package registers.
package yugabyte
