// Package parser defines the SQL parsing capability: turning statement text into
// a parse result with an opaque syntax tree. An engine provides parsing by
// implementing Parser; it is stateless and never touches a live connection.
package parser

import "context"

// AST is an intentionally opaque syntax tree. Keeping the concrete parser node
// types hidden lets the parser library be swapped without leaking those types
// across the codebase. Consumers that need the underlying value use a
// parser-specific helper inside the implementing engine package.
type AST interface{ parserAST() }

type opaqueAST struct{ value any }

func (opaqueAST) parserAST() {}

// NewOpaqueAST wraps a parser-specific tree as an opaque AST.
func NewOpaqueAST(value any) AST { return opaqueAST{value: value} }

// Parser produces a complete or partial parse of SQL text. Stateless; needs no
// connection.
type Parser interface {
	Parse(ctx context.Context, req Request) (Result, error)
}

// Request is the SQL to parse, with an optional cursor offset for editor-aware
// (incomplete-input) parsing.
type Request struct {
	SQL          string
	CursorOffset *int
}

// Result reports whether the input parsed completely, the opaque tree, and how
// many statements were found. On incomplete input Complete is false and AST
// holds a best-effort recovery tree.
type Result struct {
	Complete       bool `json:"complete"`
	AST            AST  `json:"-"`
	StatementCount int  `json:"statement_count"`
}
