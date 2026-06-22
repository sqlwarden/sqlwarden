package sqlquery

import "context"

// AST is intentionally opaque so the package can adopt or replace parser
// libraries without leaking parser-specific node types across the codebase.
type AST interface {
	sqlqueryAST()
}

// Parser produces complete or partial parse results for SQL text.
type Parser interface {
	Parse(ctx context.Context, req ParseRequest) (ParseResult, error)
}

type ParseRequest struct {
	RequestMetadata
	SQL          string
	CursorOffset *int
}

type ParseResult struct {
	Complete       bool         `json:"complete"`
	AST            AST          `json:"-"`
	StatementCount int          `json:"statement_count"`
	Diagnostics    []Diagnostic `json:"diagnostics,omitempty"`
}
