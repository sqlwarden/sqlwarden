package sqlserver

import (
	"context"
	"errors"

	"github.com/bytebase/omni/mssql/ast"

	"github.com/sqlwarden/internal/engine/parser"
	"github.com/sqlwarden/internal/engine/safety"
)

var _ safety.Checker = (*Driver)(nil)

func (d *Driver) Check(ctx context.Context, req safety.Request) (safety.Result, error) {
	statements, spans, err := parseSQLServer(ctx, req.SQL)
	if err != nil {
		var syntaxErr *parser.SyntaxError
		if errors.As(err, &syntaxErr) {
			return safety.Result{Source: "omni"}, nil
		}
		return safety.Result{}, err
	}

	var unsafe []safety.UnsafeStatement
	for i, statement := range statements {
		if sqlServerStatementMissingWhere(statement.AST) {
			unsafe = append(unsafe, safety.UnsafeStatement{
				Kind:        safety.KindUnsafeMissingWhere,
				StartOffset: spans[i].StartOffset,
				EndOffset:   spans[i].EndOffset,
			})
		}
	}
	return safety.Result{Unsafe: len(unsafe) > 0, Statements: unsafe, Source: "omni"}, nil
}

func sqlServerStatementMissingWhere(node ast.Node) bool {
	switch n := node.(type) {
	case *ast.UpdateStmt:
		return n.WhereClause == nil
	case *ast.DeleteStmt:
		return n.WhereClause == nil
	default:
		return false
	}
}
