// Package dbengine is the database-engine abstraction at the heart of the IDE.
//
// An engine is a single database integration (PostgreSQL, MySQL, SQLite, …),
// implemented as one Go type that satisfies the required Driver interface
// (connection: Connect/Ping/Close/Query/Execute/Dialect) plus whichever optional
// capability interfaces it supports — classifier.Classifier, parser.Parser,
// rewriter.Rewriter, completer.Completer, schema.SchemaInspector,
// cursor.QueryCursorDriver. Capabilities are resolved by type assertion, so an
// engine advertises a feature simply by implementing its interface; there is no
// separate capability declaration to keep in sync.
//
// Engines self-register once (from their package init) via Register. New returns
// a fresh, non-connected driver for an engine by name — call Connect on it for a
// live session, or assert a capability interface for connectionless features
// such as classification. Describe and Engines report an engine's static
// capabilities without opening a connection. The concrete engine implementations
// live under engines/<name>.
package dbengine

// EngineID is the canonical identifier for a database engine (e.g. "postgres").
type EngineID string

// EngineDescriptor is the static identity of an engine, safe to serialize and
// to report without opening a target connection.
type EngineDescriptor struct {
	ID          EngineID `json:"id"`
	DisplayName string   `json:"display_name"`
	Dialect     Dialect  `json:"dialect"`
}
