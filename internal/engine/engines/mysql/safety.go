package mysql

import (
	"context"
	"errors"

	"github.com/bytebase/omni/mysql/ast"

	"github.com/sqlwarden/internal/engine/parser"
	"github.com/sqlwarden/internal/engine/safety"
)

var _ safety.Checker = (*mysqlDriver)(nil)

func (d *mysqlDriver) Check(ctx context.Context, req safety.Request) (safety.Result, error) {
	tree, spans, err := parseMySQL(ctx, req.SQL)
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
	for i, node := range tree.Items {
		if mysqlStatementMissingWhere(node) {
			unsafe = append(unsafe, safety.UnsafeStatement{
				Kind:        safety.KindUnsafeMissingWhere,
				StartOffset: spans[i].StartOffset,
				EndOffset:   spans[i].EndOffset,
			})
		}
	}
	return safety.Result{Unsafe: len(unsafe) > 0, Statements: unsafe, Source: "omni"}, nil
}

func mysqlStatementMissingWhere(node ast.Node) bool {
	switch n := node.(type) {
	case *ast.UpdateStmt:
		return n.Where == nil
	case *ast.DeleteStmt:
		return n.Where == nil
	default:
		return false
	}
}
