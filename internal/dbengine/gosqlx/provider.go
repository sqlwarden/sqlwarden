package gosqlx

import (
	"context"
	"strings"

	gosqlxlib "github.com/ajitpratap0/GoSQLX/pkg/gosqlx"
	gosqlxast "github.com/ajitpratap0/GoSQLX/pkg/sql/ast"
	"github.com/ajitpratap0/GoSQLX/pkg/sql/keywords"
	"github.com/ajitpratap0/GoSQLX/pkg/transform"

	"github.com/sqlwarden/internal/dbengine/classifier"
	"github.com/sqlwarden/internal/dbengine/parser"
	"github.com/sqlwarden/internal/dbengine/rewriter"
	"github.com/sqlwarden/internal/driver"
)

// Classify is a GoSQLX-backed classification helper. Drivers call it from their
// own classifier.Classifier method, passing their dialect.
func Classify(ctx context.Context, dialect driver.Dialect, req classifier.Request) (classifier.Result, error) {
	if hasMySQLVersionedComment(dialect, req.SQL) {
		return classifier.Result{Kind: classifier.KindUnknown, Source: "gosqlx"}, nil
	}
	tree, err := parseWithDialect(ctx, req.SQL, dialect)
	if err != nil {
		return classifier.Result{Kind: classifier.KindUnknown, Source: "gosqlx"}, nil
	}
	return classifier.Result{Kind: classifyStatements(tree.Statements), Source: "gosqlx"}, nil
}

// Parse is a GoSQLX-backed parse helper. Drivers call it from their own
// parser.Parser method, passing their dialect.
func Parse(ctx context.Context, dialect driver.Dialect, req parser.Request) (parser.Result, error) {
	tree, err := parseWithDialect(ctx, req.SQL, dialect)
	if err == nil {
		return parser.Result{Complete: true, AST: parser.NewOpaqueAST(tree), StatementCount: len(tree.Statements)}, nil
	}
	stmts, _ := gosqlxlib.ParseWithRecovery(req.SQL)
	return parser.Result{Complete: false, AST: parser.NewOpaqueAST(stmts), StatementCount: len(stmts)}, nil
}

// Rewrite is a GoSQLX-backed pagination rewrite helper. Drivers call it from
// their own rewriter.Rewriter method, passing their dialect.
func Rewrite(ctx context.Context, dialect driver.Dialect, req rewriter.Request) (rewriter.Result, error) {
	if req.Purpose != rewriter.PurposePagination {
		return rewriteNotApplied(req.SQL, "unsupported rewrite purpose"), nil
	}
	if req.Limit < 0 || req.Offset < 0 {
		return rewriteNotApplied(req.SQL, "limit and offset must be non-negative"), nil
	}
	tree, err := parseWithDialect(ctx, req.SQL, dialect)
	if err != nil {
		return rewriteNotApplied(req.SQL, "parse failed"), nil
	}
	if len(tree.Statements) != 1 {
		return rewriteNotApplied(req.SQL, "only single statements can be rewritten"), nil
	}
	selectStmt, ok := tree.Statements[0].(*gosqlxast.SelectStatement)
	if !ok {
		return rewriteNotApplied(req.SQL, "only select statements can be rewritten"), nil
	}
	if selectHasSideEffects(selectStmt) {
		return rewriteNotApplied(req.SQL, "select statement is not safe to rewrite"), nil
	}
	if err := transform.Apply(tree.Statements[0], transform.SetLimit(req.Limit), transform.SetOffset(req.Offset)); err != nil {
		return rewriteNotApplied(req.SQL, err.Error()), nil
	}
	return rewriter.Result{SQL: transform.FormatSQLWithDialect(tree.Statements[0], toGoSQLXDialect(dialect)), Applied: true}, nil
}

func rewriteNotApplied(sql, reason string) rewriter.Result {
	return rewriter.Result{SQL: sql, Applied: false, Reason: reason}
}

func parseWithDialect(ctx context.Context, sql string, dialect driver.Dialect) (*gosqlxast.AST, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tree, err := gosqlxlib.ParseWithDialect(sql, toGoSQLXDialect(dialect))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return tree, nil
}

func toGoSQLXDialect(dialect driver.Dialect) keywords.SQLDialect {
	switch dialect {
	case driver.DialectPostgres:
		return keywords.DialectPostgreSQL
	case driver.DialectMySQL:
		return keywords.DialectMySQL
	case driver.DialectSQLite:
		return keywords.DialectSQLite
	default:
		return keywords.DialectGeneric
	}
}

func classifyStatements(stmts []gosqlxast.Statement) classifier.Kind {
	kind := classifier.KindUnknown
	for _, stmt := range stmts {
		kind = maxRisk(kind, classifyStatement(stmt))
	}
	return kind
}

func classifyStatement(stmt gosqlxast.Statement) classifier.Kind {
	if stmtHasSideEffects(stmt) {
		return classifier.KindUnknown
	}
	switch stmt.(type) {
	case *gosqlxast.SelectStatement, *gosqlxast.ShowStatement, *gosqlxast.DescribeStatement, *gosqlxast.PragmaStatement:
		return classifier.KindDQL
	case *gosqlxast.InsertStatement, *gosqlxast.UpdateStatement, *gosqlxast.DeleteStatement, *gosqlxast.MergeStatement, *gosqlxast.ReplaceStatement:
		return classifier.KindDML
	case *gosqlxast.CreateTableStatement, *gosqlxast.CreateViewStatement, *gosqlxast.CreateMaterializedViewStatement, *gosqlxast.CreateIndexStatement,
		*gosqlxast.AlterStatement, *gosqlxast.AlterTableStatement, *gosqlxast.DropStatement, *gosqlxast.TruncateStatement, *gosqlxast.RefreshMaterializedViewStatement,
		*gosqlxast.CreateSequenceStatement, *gosqlxast.DropSequenceStatement, *gosqlxast.AlterSequenceStatement:
		return classifier.KindDDL
	default:
		return classifier.KindUnknown
	}
}

func stmtHasSideEffects(stmt gosqlxast.Statement) bool {
	switch s := stmt.(type) {
	case *gosqlxast.SelectStatement:
		return selectHasSideEffects(s)
	case *gosqlxast.PragmaStatement:
		return pragmaHasSideEffects(s)
	default:
		return false
	}
}

func selectHasSideEffects(stmt *gosqlxast.SelectStatement) bool {
	if stmt == nil {
		return false
	}
	if stmt.For != nil {
		return true
	}
	if stmt.With == nil {
		return false
	}
	for _, cte := range stmt.With.CTEs {
		if classifyStatement(cte.Statement) != classifier.KindDQL {
			return true
		}
	}
	return false
}

func pragmaHasSideEffects(stmt *gosqlxast.PragmaStatement) bool {
	if stmt == nil {
		return false
	}
	return strings.TrimSpace(stmt.Value) != ""
}

func hasMySQLVersionedComment(dialect driver.Dialect, sql string) bool {
	return dialect == driver.DialectMySQL && strings.Contains(sql, "/*!")
}

func maxRisk(left, right classifier.Kind) classifier.Kind {
	if riskRank(right) > riskRank(left) {
		return right
	}
	return left
}

func riskRank(kind classifier.Kind) int {
	switch kind {
	case classifier.KindDDL:
		return 3
	case classifier.KindDML:
		return 2
	case classifier.KindDQL:
		return 1
	default:
		return 0
	}
}
