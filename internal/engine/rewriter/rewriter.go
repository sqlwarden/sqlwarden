// Package rewriter defines the SQL rewriting capability: safely transforming a
// statement for a specific purpose, such as wrapping a SELECT for server-side
// pagination. An engine provides rewriting by implementing Rewriter; it is
// stateless and never touches a live connection, and it refuses to rewrite
// anything it cannot prove safe.
package rewriter

import "context"

// Purpose identifies why a rewrite is requested; a rewriter only applies
// transformations it recognizes and considers safe for that purpose.
type Purpose string

// PurposePagination wraps a single read-only SELECT with LIMIT/OFFSET.
const PurposePagination Purpose = "pagination"

// Rewriter transforms SQL when it can do so safely for the requested Purpose.
// Stateless; needs no connection.
type Rewriter interface {
	Rewrite(ctx context.Context, req Request) (Result, error)
}

// Request is the SQL to rewrite plus the pagination bounds (used when Purpose is
// PurposePagination).
type Request struct {
	SQL     string
	Purpose Purpose
	Limit   int
	Offset  int
}

// Result holds the (possibly unchanged) SQL. Applied reports whether a
// transformation was made; when false, SQL is the original input and Reason
// explains why it was left untouched (e.g. "only select statements can be
// rewritten").
type Result struct {
	SQL     string `json:"sql"`
	Applied bool   `json:"applied"`
	Reason  string `json:"reason,omitempty"`
}
