package postgres

import (
	"context"
	"errors"

	"github.com/bytebase/omni/pg/ast"

	"github.com/sqlwarden/internal/engine/parser"
	"github.com/sqlwarden/internal/engine/safety"
)

var _ safety.Checker = (*postgresDriver)(nil)

func (d *postgresDriver) Check(ctx context.Context, req safety.Request) (safety.Result, error) {
	statements, spans, err := parsePostgres(ctx, req.SQL)
	if err != nil {
		var syntaxErr *parser.SyntaxError
		if errors.As(err, &syntaxErr) {
			// An unparseable statement cannot be executed anyway, so it is not
			// this checker's job to flag it — Classify/execution rejects it first.
			return safety.Result{Source: "omni"}, nil
		}
		return safety.Result{}, err
	}

	var unsafe []safety.UnsafeStatement
	for i, statement := range statements {
		if postgresStatementMissingWhere(statement.AST) {
			unsafe = append(unsafe, safety.UnsafeStatement{
				Kind:        safety.KindUnsafeMissingWhere,
				StartOffset: spans[i].StartOffset,
				EndOffset:   spans[i].EndOffset,
			})
		}
	}
	return safety.Result{Unsafe: len(unsafe) > 0, Statements: unsafe, Source: "omni"}, nil
}

func postgresStatementMissingWhere(node ast.Node) bool {
	switch n := node.(type) {
	case *ast.UpdateStmt:
		return n.WhereClause == nil
	case *ast.DeleteStmt:
		return n.WhereClause == nil
	default:
		return false
	}
}
