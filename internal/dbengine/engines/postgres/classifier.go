package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/bytebase/omni/pg/ast"

	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/dbengine/parser"
)

var _ classifier.Classifier = (*postgresDriver)(nil)

func (d *postgresDriver) Classify(ctx context.Context, req classifier.Request) (classifier.Result, error) {
	statements, _, err := parsePostgres(ctx, req.SQL)
	if err != nil {
		var syntaxErr *parser.SyntaxError
		if errors.As(err, &syntaxErr) {
			return classifier.Result{Kind: classifier.KindUnknown, Source: "omni"}, nil
		}
		return classifier.Result{}, err
	}

	kind := classifier.KindUnknown
	for i, statement := range statements {
		statementKind := classifyPostgresNode(statement.AST)
		if i == 0 {
			kind = statementKind
		} else {
			kind = combineKinds(kind, statementKind)
		}
	}
	return classifier.Result{
		Kind:           kind,
		Source:         "omni",
		StatementCount: len(statements),
	}, nil
}

func classifyPostgresNode(node ast.Node) classifier.Kind {
	switch n := node.(type) {
	case *ast.SelectStmt:
		return classifyPostgresSelect(n)
	case *ast.VariableShowStmt:
		return classifier.KindDQL
	case *ast.ExplainStmt:
		if postgresExplainAnalyze(n) {
			return classifyPostgresNode(n.Query)
		}
		return classifier.KindDQL
	case *ast.InsertStmt, *ast.UpdateStmt, *ast.DeleteStmt, *ast.MergeStmt,
		*ast.CopyStmt, *ast.RefreshMatViewStmt:
		return classifier.KindDML
	case *ast.CreateStmt, *ast.DropStmt, *ast.AlterTableStmt,
		*ast.CreateTableAsStmt, *ast.CreateSeqStmt, *ast.CreateSchemaStmt,
		*ast.CreatedbStmt, *ast.CreateFunctionStmt, *ast.IndexStmt,
		*ast.CreateExtensionStmt, *ast.CreateTrigStmt, *ast.CreateEventTrigStmt,
		*ast.CreateDomainStmt, *ast.CreateConversionStmt, *ast.CreateCastStmt,
		*ast.CreateOpClassStmt, *ast.CreateOpFamilyStmt, *ast.CreatePolicyStmt,
		*ast.CreateAmStmt, *ast.CreateTransformStmt, *ast.CreateStatsStmt,
		*ast.CreateTableSpaceStmt, *ast.CreateFdwStmt,
		*ast.CreateForeignServerStmt, *ast.CreateForeignTableStmt,
		*ast.CreatePLangStmt, *ast.ViewStmt, *ast.AlterSeqStmt,
		*ast.AlterDatabaseStmt, *ast.AlterDatabaseSetStmt,
		*ast.AlterFunctionStmt, *ast.AlterCollationStmt,
		*ast.AlterDomainStmt, *ast.AlterExtensionStmt,
		*ast.AlterExtensionContentsStmt, *ast.AlterFdwStmt,
		*ast.AlterForeignServerStmt, *ast.AlterOpFamilyStmt,
		*ast.AlterEventTrigStmt, *ast.AlterObjectDependsStmt,
		*ast.AlterObjectSchemaStmt, *ast.AlterOwnerStmt,
		*ast.AlterOperatorStmt, *ast.AlterTypeStmt, *ast.AlterEnumStmt,
		*ast.AlterStatsStmt, *ast.AlterTableSpaceOptionsStmt,
		*ast.CompositeTypeStmt, *ast.AlterTSConfigurationStmt,
		*ast.AlterTSDictionaryStmt, *ast.DropdbStmt, *ast.TruncateStmt,
		*ast.CommentStmt, *ast.RenameStmt, *ast.SecLabelStmt,
		*ast.RuleStmt, *ast.ReindexStmt:
		return classifier.KindDDL
	default:
		// TODO: Classify successfully parsed statements that currently fall
		// through to Unknown once their permission semantics are defined.
		return classifier.KindUnknown
	}
}

func classifyPostgresSelect(statement *ast.SelectStmt) classifier.Kind {
	if statement == nil {
		return classifier.KindUnknown
	}
	kind := classifier.KindDQL
	ast.Inspect(statement, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.SelectStmt:
			if n.IntoClause != nil {
				kind = combineKinds(kind, classifier.KindDDL)
			}
			if n.LockingClause != nil && n.LockingClause.Len() > 0 {
				kind = classifier.KindUnknown
			}
		case *ast.InsertStmt, *ast.UpdateStmt, *ast.DeleteStmt, *ast.MergeStmt,
			*ast.CopyStmt, *ast.RefreshMatViewStmt:
			kind = combineKinds(kind, classifier.KindDML)
		}
		return kind != classifier.KindUnknown
	})
	return kind
}

func postgresExplainAnalyze(statement *ast.ExplainStmt) bool {
	if statement == nil || statement.Options == nil {
		return false
	}
	for _, item := range statement.Options.Items {
		if option, ok := item.(*ast.DefElem); ok && strings.EqualFold(option.Defname, "analyze") {
			return true
		}
	}
	return false
}

func combineKinds(left, right classifier.Kind) classifier.Kind {
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
