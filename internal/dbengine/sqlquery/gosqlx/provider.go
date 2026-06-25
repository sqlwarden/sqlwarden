package gosqlx

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gosqlxlib "github.com/ajitpratap0/GoSQLX/pkg/gosqlx"
	gosqlxast "github.com/ajitpratap0/GoSQLX/pkg/sql/ast"
	"github.com/ajitpratap0/GoSQLX/pkg/sql/keywords"
	"github.com/ajitpratap0/GoSQLX/pkg/transform"
	"github.com/sqlwarden/internal/dbengine/sqlquery"
	"github.com/sqlwarden/internal/driver"
)

// NewClassifier returns a GoSQLX-backed classifier. Engines compose these
// constructors in their own sql.go; the gosqlx package no longer self-registers.
func NewClassifier() sqlquery.Classifier { return classifier{} }

// NewParser returns a GoSQLX-backed parser.
func NewParser() sqlquery.Parser { return parser{} }

// NewRewriter returns a GoSQLX-backed pagination rewriter.
func NewRewriter() sqlquery.Rewriter { return rewriter{} }

type classifier struct{}

func (classifier) Classify(ctx context.Context, req sqlquery.ClassifyRequest) (sqlquery.ClassifyResult, error) {
	if hasMySQLVersionedComment(req.Dialect, req.SQL) {
		return sqlquery.ClassifyResult{
			Kind:        sqlquery.KindUnknown,
			Diagnostics: []sqlquery.Diagnostic{{Message: "mysql versioned comments require conn:execute", Severity: "warning"}},
			Source:      "gosqlx",
		}, nil
	}

	tree, err := parseWithDialect(ctx, req.SQL, req.Dialect)
	if err != nil {
		return sqlquery.ClassifyResult{
			Kind:        sqlquery.KindUnknown,
			Diagnostics: diagnosticsFromError(err),
			Source:      "gosqlx",
		}, nil
	}

	return sqlquery.ClassifyResult{
		Kind:   classifyStatements(tree.Statements),
		Source: "gosqlx",
	}, nil
}

type parser struct{}

func (parser) Parse(ctx context.Context, req sqlquery.ParseRequest) (sqlquery.ParseResult, error) {
	tree, err := parseWithDialect(ctx, req.SQL, req.Dialect)
	if err == nil {
		return sqlquery.ParseResult{
			Complete:       true,
			AST:            sqlquery.NewOpaqueAST(tree),
			StatementCount: len(tree.Statements),
		}, nil
	}

	stmts, recoveryErrs := gosqlxlib.ParseWithRecovery(req.SQL)
	diagnostics := diagnosticsFromError(err)
	for _, recoveryErr := range recoveryErrs {
		diagnostics = append(diagnostics, diagnosticsFromError(recoveryErr)...)
	}
	return sqlquery.ParseResult{
		Complete:       false,
		AST:            sqlquery.NewOpaqueAST(stmts),
		StatementCount: len(stmts),
		Diagnostics:    diagnostics,
	}, nil
}

type rewriter struct{}

func (rewriter) Rewrite(ctx context.Context, req sqlquery.RewriteRequest) (sqlquery.RewriteResult, error) {
	if req.Purpose != sqlquery.RewritePurposePagination {
		return rewriteNotApplied(req.SQL, "unsupported rewrite purpose"), nil
	}
	if req.Limit < 0 || req.Offset < 0 {
		return rewriteNotApplied(req.SQL, "limit and offset must be non-negative"), nil
	}

	tree, err := parseWithDialect(ctx, req.SQL, req.Dialect)
	if err != nil {
		return sqlquery.RewriteResult{
			SQL:         req.SQL,
			Applied:     false,
			Reason:      "parse failed",
			Diagnostics: diagnosticsFromError(err),
		}, nil
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

	if err := transform.Apply(tree.Statements[0],
		transform.SetLimit(req.Limit),
		transform.SetOffset(req.Offset),
	); err != nil {
		return rewriteNotApplied(req.SQL, err.Error()), nil
	}

	return sqlquery.RewriteResult{
		SQL:     transform.FormatSQLWithDialect(tree.Statements[0], toGoSQLXDialect(req.Dialect)),
		Applied: true,
	}, nil
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

func classifyStatements(stmts []gosqlxast.Statement) sqlquery.Kind {
	kind := sqlquery.KindUnknown
	for _, stmt := range stmts {
		kind = maxRisk(kind, classifyStatement(stmt))
	}
	return kind
}

func classifyStatement(stmt gosqlxast.Statement) sqlquery.Kind {
	if stmtHasSideEffects(stmt) {
		return sqlquery.KindUnknown
	}

	switch stmt.(type) {
	case *gosqlxast.SelectStatement, *gosqlxast.ShowStatement, *gosqlxast.DescribeStatement, *gosqlxast.PragmaStatement:
		return sqlquery.KindDQL
	case *gosqlxast.InsertStatement, *gosqlxast.UpdateStatement, *gosqlxast.DeleteStatement, *gosqlxast.MergeStatement, *gosqlxast.ReplaceStatement:
		return sqlquery.KindDML
	case *gosqlxast.CreateTableStatement, *gosqlxast.CreateViewStatement, *gosqlxast.CreateMaterializedViewStatement, *gosqlxast.CreateIndexStatement,
		*gosqlxast.AlterStatement, *gosqlxast.AlterTableStatement, *gosqlxast.DropStatement, *gosqlxast.TruncateStatement, *gosqlxast.RefreshMaterializedViewStatement,
		*gosqlxast.CreateSequenceStatement, *gosqlxast.DropSequenceStatement, *gosqlxast.AlterSequenceStatement:
		return sqlquery.KindDDL
	default:
		return sqlquery.KindUnknown
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
		if classifyStatement(cte.Statement) != sqlquery.KindDQL {
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

func maxRisk(left, right sqlquery.Kind) sqlquery.Kind {
	if riskRank(right) > riskRank(left) {
		return right
	}
	return left
}

func riskRank(kind sqlquery.Kind) int {
	switch kind {
	case sqlquery.KindDDL:
		return 3
	case sqlquery.KindDML:
		return 2
	case sqlquery.KindDQL:
		return 1
	default:
		return 0
	}
}

func diagnosticsFromError(err error) []sqlquery.Diagnostic {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return []sqlquery.Diagnostic{{Message: err.Error(), Severity: "error"}}
	}
	return []sqlquery.Diagnostic{{Message: fmt.Sprintf("%v", err), Severity: "error"}}
}

func rewriteNotApplied(sql, reason string) sqlquery.RewriteResult {
	return sqlquery.RewriteResult{
		SQL:     sql,
		Applied: false,
		Reason:  reason,
	}
}
