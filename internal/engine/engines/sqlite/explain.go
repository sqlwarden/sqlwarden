package sqlite

import (
	"context"
	"fmt"

	rqlitesql "github.com/rqlite/sql"

	"github.com/sqlwarden/internal/engine/explain"
)

var _ explain.Explainer = (*sqliteDriver)(nil)

var sqliteExplainSpec = explain.Spec{SupportsAnalyze: false}

func (*sqliteDriver) ExplainSpec() explain.Spec { return sqliteExplainSpec }

// Explain wraps sql with SQLite's EXPLAIN QUERY PLAN. SQLite has no ANALYZE
// variant that reports execution timing per plan node.
func (*sqliteDriver) Explain(sql string, mode explain.Mode) (explain.Plan, error) {
	if err := validateExplainable(sql); err != nil {
		return explain.Plan{}, err
	}
	switch mode {
	case explain.ModePlain:
		return explain.Plan{Statement: fmt.Sprintf("EXPLAIN QUERY PLAN %s", sql)}, nil
	case explain.ModeAnalyze:
		return explain.Plan{}, explain.ErrAnalyzeUnsupported
	default:
		return explain.Plan{}, explain.ErrUnsupported
	}
}

// validateExplainable checks that sql is exactly one statement and not already
// an EXPLAIN. A parse failure is left for the database to report.
func validateExplainable(sql string) error {
	stmts, _, err := parseSQLite(context.Background(), sql)
	if err != nil {
		return nil
	}
	if len(stmts) != 1 {
		return explain.ErrMultipleStatements
	}
	if _, ok := stmts[0].(*rqlitesql.ExplainStatement); ok {
		return explain.ErrAlreadyExplained
	}
	return nil
}
