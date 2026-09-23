package safety

import "context"

// Kind identifies a specific unsafe-statement rule.
type Kind string

const (
	// KindUnsafeMissingWhere flags an UPDATE or DELETE with no WHERE clause.
	KindUnsafeMissingWhere Kind = "unsafe_missing_where"
)

// Request is the SQL to check.
type Request struct {
	SQL string
}

// UnsafeStatement identifies one statement within the request that failed a
// safety rule, by its byte range in the original SQL.
type UnsafeStatement struct {
	Kind        Kind `json:"kind"`
	StartOffset int  `json:"start_offset"`
	EndOffset   int  `json:"end_offset"`
}

// Result is the safety-check outcome. A multi-statement script is checked
// statement-by-statement: Unsafe is true if any statement is unsafe, and
// Statements lists every unsafe statement found.
type Result struct {
	Unsafe     bool              `json:"unsafe"`
	Statements []UnsafeStatement `json:"statements,omitempty"`
	Source     string            `json:"source,omitempty"` // "omni" | "heuristic"
}

// Checker determines whether SQL contains statements that require explicit
// user confirmation before execution. It is stateless and does not require a
// live connection, mirroring classifier.Classifier.
type Checker interface {
	Check(ctx context.Context, req Request) (Result, error)
}
