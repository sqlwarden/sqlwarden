package sqlserver

import (
	"context"

	"github.com/sqlwarden/internal/engine/explain"
)

var _ explain.Explainer = (*Driver)(nil)

var sqlServerExplainSpec = explain.Spec{SupportsAnalyze: false}

func (*Driver) ExplainSpec() explain.Spec { return sqlServerExplainSpec }

// Explain wraps sql with SET SHOWPLAN_XML ON as Setup and OFF as Teardown,
// since it is a session-level toggle: leaving it on would make every later
// statement on the same connection return a plan instead of executing, so
// Teardown must always run even if Statement fails.
//
// EXPLAIN ANALYZE (ModeAnalyze) is intentionally unsupported: under
// SET STATISTICS XML ON, SQL Server returns the statement's own result set
// first and the showplan XML as a *second* result set. Nothing in this
// codebase's cursor/explain path advances to a later result set, so
// enabling analyze here would silently render the query's own output
// mislabeled as a plan. Fixing this properly requires teaching the shared
// cursor/explain contract to drain to a second result set, which affects
// every engine, not just SQL Server — do not just flip SupportsAnalyze
// back to true without doing that first.
func (*Driver) Explain(sql string, mode explain.Mode) (explain.Plan, error) {
	if err := validateSQLServerExplainable(sql); err != nil {
		return explain.Plan{}, err
	}
	switch mode {
	case explain.ModePlain:
		return explain.Plan{
			Setup:     []string{"SET SHOWPLAN_XML ON"},
			Statement: sql,
			Teardown:  []string{"SET SHOWPLAN_XML OFF"},
		}, nil
	case explain.ModeAnalyze:
		return explain.Plan{}, explain.ErrAnalyzeUnsupported
	default:
		return explain.Plan{}, explain.ErrUnsupported
	}
}

// validateSQLServerExplainable checks that sql is exactly one statement. A
// parse failure is left for the target database to report. T-SQL has no
// EXPLAIN/DESCRIBE statement node in the omni AST (confirmed absent from
// ast/parsenodes.go's statement list), so there is no ErrAlreadyExplained
// case to detect here, unlike mysql's ast.ExplainStmt check.
func validateSQLServerExplainable(sql string) error {
	statements, _, err := parseSQLServer(context.Background(), sql)
	if err != nil || statements == nil {
		return nil
	}
	if len(statements) != 1 {
		return explain.ErrMultipleStatements
	}
	return nil
}
