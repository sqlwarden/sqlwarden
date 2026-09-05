package mysql

import (
	"context"
	"fmt"

	"github.com/bytebase/omni/mysql/ast"

	"github.com/sqlwarden/internal/engine/explain"
)

var _ explain.Explainer = (*Driver)(nil)

var mysqlExplainSpec = explain.Spec{SupportsAnalyze: true}

func (*Driver) ExplainSpec() explain.Spec { return mysqlExplainSpec }

func (*Driver) Explain(sql string, mode explain.Mode) (explain.Plan, error) {
	if err := validateExplainable(sql); err != nil {
		return explain.Plan{}, err
	}
	switch mode {
	case explain.ModePlain:
		return explain.Plan{Statement: fmt.Sprintf("EXPLAIN %s", sql)}, nil
	case explain.ModeAnalyze:
		return explain.Plan{Statement: fmt.Sprintf("EXPLAIN ANALYZE %s", sql)}, nil
	default:
		return explain.Plan{}, explain.ErrUnsupported
	}
}

// validateExplainable checks that sql is exactly one statement and not
// already an EXPLAIN (or its DESCRIBE/DESC synonyms, which MySQL parses as
// the same node), returning explain.ErrMultipleStatements or
// explain.ErrAlreadyExplained respectively. A parse failure is left for the
// target database to report, so it does not fail validation here.
func validateExplainable(sql string) error {
	tree, _, err := parseMySQL(context.Background(), sql)
	if err != nil || tree == nil {
		return nil
	}
	if len(tree.Items) != 1 {
		return explain.ErrMultipleStatements
	}
	if _, ok := tree.Items[0].(*ast.ExplainStmt); ok {
		return explain.ErrAlreadyExplained
	}
	return nil
}
