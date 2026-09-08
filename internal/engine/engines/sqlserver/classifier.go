package sqlserver

import (
	"context"
	"errors"

	"github.com/bytebase/omni/mssql/ast"

	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/engine/parser"
)

var _ classifier.Classifier = (*Driver)(nil)

func (d *Driver) Classify(ctx context.Context, req classifier.Request) (classifier.Result, error) {
	statements, _, err := parseSQLServer(ctx, req.SQL)
	if err != nil {
		var syntaxErr *parser.SyntaxError
		if errors.As(err, &syntaxErr) {
			return classifier.Result{Kind: classifier.KindUnknown, Source: "omni"}, nil
		}
		return classifier.Result{}, err
	}

	kind := classifier.KindUnknown
	for i, statement := range statements {
		statementKind := classifySQLServerNode(statement.AST)
		if i == 0 {
			kind = statementKind
		} else {
			kind = combineSQLServerKinds(kind, statementKind)
		}
	}
	return classifier.Result{
		Kind:           kind,
		Source:         "omni",
		StatementCount: len(statements),
	}, nil
}

func classifySQLServerNode(node ast.Node) classifier.Kind {
	switch node.(type) {
	case *ast.SelectStmt:
		return classifier.KindDQL
	case *ast.InsertStmt, *ast.UpdateStmt, *ast.DeleteStmt, *ast.MergeStmt, *ast.BulkInsertStmt:
		return classifier.KindDML
	case *ast.CreateTableStmt, *ast.AlterTableStmt, *ast.DropStmt,
		*ast.CreateIndexStmt, *ast.AlterIndexStmt,
		*ast.CreateViewStmt, *ast.CreateTriggerStmt,
		*ast.CreateFunctionStmt, *ast.CreateProcedureStmt,
		*ast.CreateDatabaseStmt, *ast.AlterDatabaseStmt,
		*ast.CreateSchemaStmt, *ast.AlterSchemaStmt,
		*ast.CreateSequenceStmt, *ast.AlterSequenceStmt,
		*ast.RenameStmt, *ast.TruncateStmt:
		return classifier.KindDDL
	default:
		return classifier.KindUnknown
	}
}

func combineSQLServerKinds(left, right classifier.Kind) classifier.Kind {
	if left == classifier.KindUnknown || right == classifier.KindUnknown {
		return classifier.KindUnknown
	}
	if left == classifier.KindDQL {
		return right
	}
	if right == classifier.KindDQL || left == right {
		return left
	}
	return classifier.KindUnknown
}
