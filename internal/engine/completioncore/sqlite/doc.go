// Package sqlite adapts rqlite/sql's scanner to SQLWarden's dialect-neutral
// completion boundary. rqlite/sql exposes no ANTLR candidate collector, so
// cursor intent is derived from a backward token scan and every name is
// resolved from SQLWarden's own metadata index via
// completioncore.MetadataResolver. No completer opens a live connection.
package sqlite
