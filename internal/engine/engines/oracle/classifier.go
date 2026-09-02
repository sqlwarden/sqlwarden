package oracle

import (
	"context"
	"errors"

	"github.com/bytebase/omni/oracle/ast"

	"github.com/sqlwarden/internal/engine/classifier"
	"github.com/sqlwarden/internal/engine/parser"
)

var _ classifier.Classifier = (*oracleDriver)(nil)

func (d *oracleDriver) Classify(ctx context.Context, req classifier.Request) (classifier.Result, error) {
	statements, _, err := parseOracle(ctx, req.SQL)
	if err != nil {
		var syntaxErr *parser.SyntaxError
		if errors.As(err, &syntaxErr) {
			return classifier.Result{Kind: classifier.KindUnknown, Source: "omni"}, nil
		}
		return classifier.Result{}, err
	}

	kind := classifier.KindUnknown
	for i, stmt := range statements {
		statementKind := classifyOracleNode(stmt.AST)
		if i == 0 {
			kind = statementKind
		} else {
			kind = combineOracleKinds(kind, statementKind)
		}
	}
	return classifier.Result{Kind: kind, Source: "omni", StatementCount: len(statements)}, nil
}

func classifyOracleNode(node ast.Node) classifier.Kind {
	switch n := node.(type) {
	case *ast.SelectStmt:
		return classifyOracleSelect(n)
	case *ast.InsertStmt, *ast.UpdateStmt, *ast.DeleteStmt, *ast.MergeStmt:
		return classifier.KindDML
	case *ast.ExplainPlanStmt:
		if n.Statement == nil {
			return classifier.KindUnknown
		}
		return classifyOracleNode(n.Statement)
	case *ast.CreateTableStmt, *ast.CreateViewStmt, *ast.CreateIndexStmt,
		*ast.CreateSequenceStmt, *ast.CreateSynonymStmt, *ast.CreateFunctionStmt,
		*ast.CreateProcedureStmt, *ast.CreatePackageStmt, *ast.CreateTriggerStmt,
		*ast.CreateTypeStmt, *ast.CreateUserStmt, *ast.CreateRoleStmt,
		*ast.AlterTableStmt, *ast.AlterIndexStmt, *ast.AlterViewStmt,
		*ast.AlterSequenceStmt, *ast.AlterUserStmt,
		*ast.DropStmt, *ast.DropTablespaceStmt, *ast.TruncateStmt,
		*ast.RenameStmt, *ast.CommentStmt, *ast.GrantStmt, *ast.RevokeStmt,
		*ast.AnalyzeStmt, *ast.PurgeStmt, *ast.FlashbackTableStmt,
		*ast.AuditStmt, *ast.NoauditStmt:
		return classifier.KindDDL
	default:
		// Anonymous PL/SQL blocks, CALL, LOCK TABLE, session/transaction
		// control, and anything unrecognised: strictest treatment. The runtime
		// authorization layer treats Unknown as maximally privileged.
		return classifier.KindUnknown
	}
}

func classifyOracleSelect(statement *ast.SelectStmt) classifier.Kind {
	if statement == nil {
		return classifier.KindUnknown
	}
	kind := classifier.KindDQL
	ast.Inspect(statement, func(node ast.Node) bool {
		if nested, ok := node.(*ast.SelectStmt); ok && nested.ForUpdate != nil {
			kind = classifier.KindUnknown
		}
		return kind != classifier.KindUnknown
	})
	return kind
}

// combineOracleKinds folds a script's per-statement kinds. A script is DQL only
// if every statement is DQL; mixing DML/DDL, or any Unknown, yields Unknown.
func combineOracleKinds(left, right classifier.Kind) classifier.Kind {
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
