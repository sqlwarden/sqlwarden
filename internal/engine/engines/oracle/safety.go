package oracle

import (
	"context"
	"errors"

	"github.com/bytebase/omni/oracle/ast"

	"github.com/sqlwarden/internal/engine/parser"
	"github.com/sqlwarden/internal/engine/safety"
)

var _ safety.Checker = (*oracleDriver)(nil)

func (d *oracleDriver) Check(ctx context.Context, req safety.Request) (safety.Result, error) {
	statements, spans, err := parseOracle(ctx, req.SQL)
	if err != nil {
		var syntaxErr *parser.SyntaxError
		if errors.As(err, &syntaxErr) {
			// Unparseable SQL cannot execute; classification/execution rejects
			// it first, so it is not this checker's concern.
			return safety.Result{Source: "omni"}, nil
		}
		return safety.Result{}, err
	}

	var unsafe []safety.UnsafeStatement
	for i, stmt := range statements {
		if oracleStatementMissingWhere(stmt.AST) {
			unsafe = append(unsafe, safety.UnsafeStatement{
				Kind:        safety.KindUnsafeMissingWhere,
				StartOffset: spans[i].StartOffset,
				EndOffset:   spans[i].EndOffset,
			})
		}
	}
	return safety.Result{Unsafe: len(unsafe) > 0, Statements: unsafe, Source: "omni"}, nil
}

func oracleStatementMissingWhere(node ast.Node) bool {
	switch n := node.(type) {
	case *ast.UpdateStmt:
		return n.WhereClause == nil
	case *ast.DeleteStmt:
		return n.WhereClause == nil
	default:
		return false
	}
}
