// Package engine defines SQLWarden's integrations with external data systems.
//
// An engine is a single system integration implemented as one Go type that
// satisfies the required Driver interface (connection:
// Connect/Ping/Close/Query/Execute/Dialect) plus whichever optional capability
// interfaces it supports — classifier.Classifier, parser.Parser,
// safety.Checker, rewriter.Rewriter, completer.Completer, explain.Explainer,
// metadata.SchemaInspector, cursor.QueryCursorDriver, ddl.Executor,
// statement.Generator, transaction.Controller, TLSCapable, and
// SSHTunnelCapable. Capabilities are resolved by type assertion, so an engine
// advertises a feature simply by implementing its interface; there is no
// separate declaration to keep in sync.
//
// Engines self-register once (from their package init) via Register. New returns
// a fresh, non-connected driver for an engine by name — call Connect on it for a
// live session, or assert a capability interface for connectionless features
// such as classification. Describe and Engines report an engine's static
// capabilities without opening a connection.
//
// The concrete engine implementations live under engines/<name>: PostgreSQL
// and the engines that embed it (CockroachDB, Neon, Supabase, YugabyteDB),
// MySQL and the engines that embed it (MariaDB, TiDB), Oracle, SQL Server, and
// SQLite.
package engine
