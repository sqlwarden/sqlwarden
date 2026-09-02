// Package explain defines the optional engine capability for producing an
// EXPLAIN form of a single statement. An engine provides this by implementing
// Explainer; it is stateless and never touches a live connection or executes
// SQL — it only rewrites statement text into the Plan the caller then runs.
package explain

import "errors"

// ErrUnsupported means the engine has no EXPLAIN form at all.
var ErrUnsupported = errors.New("explain is not supported")

// ErrAnalyzeUnsupported means the engine can EXPLAIN but not EXPLAIN ANALYZE.
var ErrAnalyzeUnsupported = errors.New("explain analyze is not supported")

// ErrAlreadyExplained means sql's top-level statement is already an EXPLAIN
// (or dialect equivalent, e.g. MySQL's DESCRIBE/DESC), so wrapping it again
// would produce invalid nested-EXPLAIN syntax.
var ErrAlreadyExplained = errors.New("statement is already an explain statement")

// ErrMultipleStatements means sql contains more than one statement, so it
// cannot be unambiguously wrapped in a single EXPLAIN.
var ErrMultipleStatements = errors.New("explain requires exactly one statement")

// Mode selects the EXPLAIN variant. ModeAnalyze actually executes the wrapped
// statement to gather real timing/row data, unlike ModePlain which only plans it.
type Mode string

const (
	ModePlain   Mode = "plain"
	ModeAnalyze Mode = "analyze"
)

// Spec is the static EXPLAIN capability advertised by an engine.
type Spec struct {
	SupportsAnalyze bool `json:"supports_analyze"`
}

// Plan is the statement sequence a caller runs to produce an EXPLAIN result.
// Most dialects express EXPLAIN as one self-contained statement and set only
// Statement. Oracle needs "EXPLAIN PLAN FOR <stmt>" to run before the
// DBMS_XPLAN query that reads the plan back, which is what Setup/Teardown are
// for.
type Plan struct {
	// Setup holds statements run in order for their side effects before
	// Statement. Their result sets are discarded. If any fails, Statement is
	// not run.
	Setup []string
	// Statement is run last; its result set is the plan output shown to the
	// user.
	Statement string
	// Teardown holds statements run after Statement on a best-effort basis,
	// including when Statement fails, to undo session state changed by Setup
	// (e.g. ALTER SESSION). A Teardown failure is logged, not surfaced.
	Teardown []string
}

// Explainer advertises and produces a dialect-specific EXPLAIN Plan for a
// single statement. Explain is pure: it uses only the supplied SQL text and
// never executes anything or opens a connection.
type Explainer interface {
	// ExplainSpec reports which modes Explain accepts.
	ExplainSpec() Spec
	// Explain returns the Plan that produces an EXPLAIN of sql for mode.
	// Explain validates sql itself: it returns ErrMultipleStatements if sql
	// is not exactly one statement, and ErrAlreadyExplained if sql is
	// already an EXPLAIN statement. Callers do not need to pre-validate sql.
	Explain(sql string, mode Mode) (Plan, error)
}
