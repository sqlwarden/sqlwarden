// Package parser defines the SQL parsing capability: turning statement text into
// a parse result with an opaque syntax tree. An engine provides parsing by
// implementing Parser; it is stateless and never touches a live connection.
package parser
