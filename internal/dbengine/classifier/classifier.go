package classifier

import "context"

// Kind is the coarse statement class used for RBAC query-execution checks.
type Kind string

const (
	KindUnknown Kind = "unknown"
	KindDQL     Kind = "dql"
	KindDML     Kind = "dml"
	KindDDL     Kind = "ddl"
)

// Classifier determines the coarse class of SQL statements. It is stateless and
// does not require a live connection.
type Classifier interface {
	Classify(ctx context.Context, req Request) (Result, error)
}

type Request struct {
	SQL string
}

type Result struct {
	Kind   Kind   `json:"kind"`
	Source string `json:"source,omitempty"` // "gosqlx" | "heuristic"
}
