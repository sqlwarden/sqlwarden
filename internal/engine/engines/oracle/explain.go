package oracle

import (
	"context"
	"strings"

	"github.com/bytebase/omni/oracle/ast"

	"github.com/sqlwarden/internal/engine/explain"
)

var _ explain.Explainer = (*oracleDriver)(nil)

var oracleExplainSpec = explain.Spec{SupportsAnalyze: true}

func (*oracleDriver) ExplainSpec() explain.Spec { return oracleExplainSpec }

// Explain builds the statement sequence for an Oracle EXPLAIN. Unlike the other
// engines Oracle has no single self-contained EXPLAIN statement: the plan is
// written by "EXPLAIN PLAN FOR <stmt>" and read back by a separate DBMS_XPLAN
// query. PLAN_TABLE is a built-in public synonym on every supported release, so
// no table setup is needed.
//
// ModeAnalyze runs the statement for real with row-source statistics enabled,
// then reports the actual execution plan via DBMS_XPLAN.DISPLAY_CURSOR. It
// needs the connection account to have access to the V$SQL_PLAN family of
// fixed views; without it the DISPLAY_CURSOR query returns an explanatory
// message rather than a plan.
func (*oracleDriver) Explain(sql string, mode explain.Mode) (explain.Plan, error) {
	if err := validateExplainable(sql); err != nil {
		return explain.Plan{}, err
	}
	stmt := trimTrailingSemicolon(sql)
	switch mode {
	case explain.ModePlain:
		return explain.Plan{
			Setup:     []string{"EXPLAIN PLAN FOR " + stmt},
			Statement: "SELECT plan_table_output FROM TABLE(DBMS_XPLAN.DISPLAY(NULL, NULL, 'TYPICAL'))",
		}, nil
	case explain.ModeAnalyze:
		return explain.Plan{
			Setup: []string{
				"ALTER SESSION SET STATISTICS_LEVEL = ALL",
				stmt,
			},
			Statement: "SELECT plan_table_output FROM TABLE(DBMS_XPLAN.DISPLAY_CURSOR(NULL, NULL, 'ALLSTATS LAST'))",
			Teardown:  []string{"ALTER SESSION SET STATISTICS_LEVEL = TYPICAL"},
		}, nil
	default:
		return explain.Plan{}, explain.ErrUnsupported
	}
}

// validateExplainable checks that sql is exactly one statement and not already
// an EXPLAIN PLAN, returning explain.ErrMultipleStatements or
// explain.ErrAlreadyExplained respectively. A parse failure is left for the
// target database to report, so it does not fail validation here.
func validateExplainable(sql string) error {
	statements, _, err := parseOracle(context.Background(), sql)
	if err != nil {
		return nil
	}
	if len(statements) != 1 {
		return explain.ErrMultipleStatements
	}
	if _, ok := statements[0].AST.(*ast.ExplainPlanStmt); ok {
		return explain.ErrAlreadyExplained
	}
	return nil
}

// trimTrailingSemicolon removes a single trailing statement terminator and
// surrounding whitespace. go-ora rejects a trailing ";" on a lone statement,
// and "EXPLAIN PLAN FOR <stmt>;" would carry it into the wrapped text.
func trimTrailingSemicolon(sql string) string {
	trimmed := strings.TrimSpace(sql)
	trimmed = strings.TrimSuffix(trimmed, ";")
	return strings.TrimSpace(trimmed)
}
