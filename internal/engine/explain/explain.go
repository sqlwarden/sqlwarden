// Package explain defines the optional engine capability for producing an
// EXPLAIN form of a single statement. An engine provides this by implementing
// Explainer; it is stateless and never touches a live connection or executes
// SQL — it only rewrites statement text.
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

// Explainer advertises and produces a dialect-specific EXPLAIN wrapping of a
// single statement. Explain is pure: it uses only the supplied SQL text and
// never executes anything or opens a connection.
type Explainer interface {
	// ExplainSpec reports which modes Explain accepts.
	ExplainSpec() Spec
	// Explain returns sql wrapped in the engine's EXPLAIN syntax for mode.
	// Explain validates sql itself: it returns ErrMultipleStatements if sql
	// is not exactly one statement, and ErrAlreadyExplained if sql is
	// already an EXPLAIN statement. Callers do not need to pre-validate sql.
	Explain(sql string, mode Mode) (string, error)
}
