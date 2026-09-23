// Package cursor is the database/sql layer for streaming query results: the
// QueryCursor interface for forward-only, page-at-a-time reads, its default
// database/sql-backed implementation (SQLRowsCursor), and the shared row-scanning
// helpers (ScanRows, NormalizeValue) every engine uses to turn driver rows into a
// normalized result.ResultSet. An engine opts into server-side cursors by
// implementing QueryCursorDriver.
package cursor
