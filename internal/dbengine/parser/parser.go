package parser

import "context"

// AST is intentionally opaque so the parser library can change without leaking
// parser-specific node types across the codebase.
type AST interface{ parserAST() }

type opaqueAST struct{ value any }

func (opaqueAST) parserAST() {}

// NewOpaqueAST wraps a parser-specific tree without exposing its concrete types.
func NewOpaqueAST(value any) AST { return opaqueAST{value: value} }

// Parser produces complete or partial parse results. Stateless; no connection.
type Parser interface {
	Parse(ctx context.Context, req Request) (Result, error)
}

type Request struct {
	SQL          string
	CursorOffset *int
}

type Result struct {
	Complete       bool `json:"complete"`
	AST            AST  `json:"-"`
	StatementCount int  `json:"statement_count"`
}
