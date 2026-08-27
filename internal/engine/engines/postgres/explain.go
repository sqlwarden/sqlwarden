package postgres

import (
	"context"
	"fmt"

	"github.com/bytebase/omni/pg/ast"

	"github.com/sqlwarden/internal/engine/explain"
)

var _ explain.Explainer = (*postgresDriver)(nil)

var postgresExplainSpec = explain.Spec{SupportsAnalyze: true}

func (*postgresDriver) ExplainSpec() explain.Spec { return postgresExplainSpec }

func (*postgresDriver) Explain(sql string, mode explain.Mode) (string, error) {
	if err := validateExplainable(sql); err != nil {
		return "", err
	}
	switch mode {
	case explain.ModePlain:
		return fmt.Sprintf("EXPLAIN (FORMAT TEXT) %s", sql), nil
	case explain.ModeAnalyze:
		return fmt.Sprintf("EXPLAIN (ANALYZE, FORMAT TEXT) %s", sql), nil
	default:
		return "", explain.ErrUnsupported
	}
}

// validateExplainable checks that sql is exactly one statement and not
// already an EXPLAIN, returning explain.ErrMultipleStatements or
// explain.ErrAlreadyExplained respectively. A parse failure is left for the
// target database to report, so it does not fail validation here.
func validateExplainable(sql string) error {
	statements, _, err := parsePostgres(context.Background(), sql)
	if err != nil {
		return nil
	}
	if len(statements) != 1 {
		return explain.ErrMultipleStatements
	}
	if _, ok := statements[0].AST.(*ast.ExplainStmt); ok {
		return explain.ErrAlreadyExplained
	}
	return nil
}
