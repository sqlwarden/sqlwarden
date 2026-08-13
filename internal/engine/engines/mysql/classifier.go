package mysql

import (
	"context"
	"errors"

	"github.com/bytebase/omni/mysql/ast"

	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/engine/parser"
)

var _ classifier.Classifier = (*mysqlDriver)(nil)

func (d *mysqlDriver) Classify(ctx context.Context, req classifier.Request) (classifier.Result, error) {
	tree, _, err := parseMySQL(ctx, req.SQL)
	if err != nil {
		var syntaxErr *parser.SyntaxError
		if errors.As(err, &syntaxErr) {
			return classifier.Result{Kind: classifier.KindUnknown, Source: "omni"}, nil
		}
		return classifier.Result{}, err
	}

	kind := classifier.KindUnknown
	for i, node := range tree.Items {
		statementKind := classifyMySQLNode(node)
		if i == 0 {
			kind = statementKind
		} else {
			kind = combineMySQLKinds(kind, statementKind)
		}
	}
	return classifier.Result{
		Kind:           kind,
		Source:         "omni",
		StatementCount: tree.Len(),
	}, nil
}

func classifyMySQLNode(node ast.Node) classifier.Kind {
	switch n := node.(type) {
	case *ast.SelectStmt:
		return classifyMySQLSelect(n)
	case *ast.TableStmt, *ast.ValuesStmt, *ast.ShowStmt:
		return classifier.KindDQL
	case *ast.ExplainStmt:
		if n.Analyze {
			if n.Stmt == nil {
				return classifier.KindUnknown
			}
			return classifyMySQLNode(n.Stmt)
		}
		return classifier.KindDQL
	case *ast.InsertStmt, *ast.UpdateStmt, *ast.DeleteStmt, *ast.LoadDataStmt:
		return classifier.KindDML
	case *ast.CreateDatabaseStmt, *ast.CreateTableStmt, *ast.CreateIndexStmt,
		*ast.CreateViewStmt, *ast.CreateEventStmt, *ast.CreateTriggerStmt,
		*ast.CreateFunctionStmt, *ast.CreateSpatialRefSysStmt,
		*ast.AlterDatabaseStmt,
		*ast.AlterTableStmt, *ast.AlterViewStmt, *ast.AlterEventStmt,
		*ast.AlterRoutineStmt,
		*ast.DropDatabaseStmt, *ast.DropTableStmt, *ast.DropIndexStmt,
		*ast.DropViewStmt, *ast.DropEventStmt, *ast.DropTriggerStmt,
		*ast.DropRoutineStmt, *ast.DropSpatialRefSysStmt,
		*ast.RenameTableStmt, *ast.TruncateStmt:
		return classifier.KindDDL
	default:
		// TODO: Classify successfully parsed statements that currently fall
		// through to Unknown once their permission semantics are defined.
		return classifier.KindUnknown
	}
}

func classifyMySQLSelect(statement *ast.SelectStmt) classifier.Kind {
	if statement == nil {
		return classifier.KindUnknown
	}
	kind := classifier.KindDQL
	ast.Inspect(statement, func(node ast.Node) bool {
		if nested, ok := node.(*ast.SelectStmt); ok && (nested.Into != nil || nested.ForUpdate != nil) {
			kind = classifier.KindUnknown
		}
		return kind != classifier.KindUnknown
	})
	return kind
}

func combineMySQLKinds(left, right classifier.Kind) classifier.Kind {
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
