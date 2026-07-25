// Package parser defines the SQL parsing capability: turning statement text into
// a parse result with an opaque syntax tree. An engine provides parsing by
// implementing Parser; it is stateless and never touches a live connection.
package parser

import (
	"context"
	"fmt"
)

// AST is an intentionally opaque syntax tree. Keeping the concrete parser node
// types hidden lets the parser library be swapped without leaking those types
// across the codebase. Consumers that need the underlying value use a
// parser-specific helper inside the implementing engine package.
type AST interface{ parserAST() }

type opaqueAST struct{ value any }

func (opaqueAST) parserAST() {}

// NewOpaqueAST wraps a parser-specific tree as an opaque AST.
func NewOpaqueAST(value any) AST { return opaqueAST{value: value} }

// Parser strictly parses complete SQL text. It is stateless and needs no
// connection. Editor completion and recovery parsing use the separate completer
// capability.
type Parser interface {
	Parse(ctx context.Context, req Request) (Result, error)
}

// Request is the SQL to parse.
type Request struct {
	SQL string
}

// Statement identifies one parsed statement in the original SQL. StartOffset is
// inclusive, EndOffset is exclusive, and both are zero-based UTF-8 byte
// offsets. A trailing statement delimiter is excluded. Statements expanded
// from one dialect-specific executable source segment may share a range.
type Statement struct {
	StartOffset int `json:"start_offset"`
	EndOffset   int `json:"end_offset"`
}

// Result contains the opaque dialect AST and normalized statement boundaries.
type Result struct {
	AST            AST         `json:"-"`
	Statements     []Statement `json:"statements"`
	StatementCount int         `json:"statement_count"`
}

// SyntaxError is a normalized parser error. Offset is a zero-based UTF-8 byte
// offset. Line and Column are one-based; Column counts bytes so it remains
// consistent with Offset and with both supported dialect parsers.
type SyntaxError struct {
	Message string
	Offset  int
	Line    int
	Column  int
}

func (e *SyntaxError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s (line %d, column %d)", e.Message, e.Line, e.Column)
	}
	return e.Message
}

// Position converts a zero-based UTF-8 byte offset into a one-based line and
// byte column. Offsets outside sql are clamped.
func Position(sql string, offset int) (line, column int) {
	offset = ClampOffset(sql, offset)
	line, column = 1, 1
	for i := 0; i < offset; i++ {
		if sql[i] == '\n' {
			line, column = line+1, 1
		} else {
			column++
		}
	}
	return line, column
}

// ClampOffset constrains a byte offset to the bounds of sql.
func ClampOffset(sql string, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > len(sql) {
		return len(sql)
	}
	return offset
}
