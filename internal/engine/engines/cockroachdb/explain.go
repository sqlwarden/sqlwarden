package cockroachdb

import (
	"fmt"

	"github.com/bytebase/omni/pg"
	"github.com/bytebase/omni/pg/ast"

	"github.com/sqlwarden/internal/engine/explain"
)

var _ explain.Explainer = (*driver)(nil)

var cockroachdbExplainSpec = explain.Spec{SupportsAnalyze: true}

func (*driver) ExplainSpec() explain.Spec { return cockroachdbExplainSpec }

// Explain emits CockroachDB's own EXPLAIN grammar: unlike PostgreSQL,
// CockroachDB has no FORMAT TEXT option, and EXPLAIN ANALYZE is its own
// top-level statement rather than an option passed inside EXPLAIN (...).
func (*driver) Explain(sql string, mode explain.Mode) (explain.Plan, error) {
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
// already an EXPLAIN, returning explain.ErrMultipleStatements or
// explain.ErrAlreadyExplained respectively. It parses with the same
// PostgreSQL-grammar parser the embedded postgres.Driver uses (CockroachDB's
// grammar is PostgreSQL-compatible for this purpose), reimplemented locally
// because postgres.Driver's own parsing helper is unexported and its Parse
// result wraps the AST opaquely, so it cannot be recovered by an embedding
// type. A parse failure is left for the target database to report, so it
// does not fail validation here.
func validateExplainable(sql string) error {
	statements, err := pg.Parse(sql)
	if err != nil {
		return nil
	}
	var count int
	var last ast.Node
	for _, statement := range statements {
		if statement.AST == nil {
			continue
		}
		count++
		last = statement.AST
	}
	if count != 1 {
		return explain.ErrMultipleStatements
	}
	if _, ok := last.(*ast.ExplainStmt); ok {
		return explain.ErrAlreadyExplained
	}
	return nil
}
