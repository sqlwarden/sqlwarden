// Package classifier defines the SQL classification capability: determining the
// coarse class (DQL/DML/DDL) of a statement so the runtime can authorize it
// against the right connection permission. An engine provides classification by
// implementing Classifier; it is stateless and never touches a live connection.
package classifier

import "context"

// Kind is the coarse statement class used for RBAC query-execution checks.
type Kind string

const (
	// KindUnknown means the statement could not be safely classified; callers
	// must treat it as the most restrictive class.
	KindUnknown Kind = "unknown"
	// KindDQL is a read-only query (SELECT and friends).
	KindDQL Kind = "dql"
	// KindDML mutates data (INSERT/UPDATE/DELETE/…).
	KindDML Kind = "dml"
	// KindDDL changes schema (CREATE/ALTER/DROP/…).
	KindDDL Kind = "ddl"
)

// Classifier determines the coarse class of SQL statements. It is stateless and
// does not require a live connection, so it can authorize a query before the
// engine connects.
type Classifier interface {
	Classify(ctx context.Context, req Request) (Result, error)
}

// Request is the SQL to classify.
type Request struct {
	SQL string
}

// Result is the classification outcome. A multi-statement script reports the
// most privileged class found (e.g. a SELECT followed by a DROP is KindDDL).
type Result struct {
	Kind   Kind   `json:"kind"`
	Source string `json:"source,omitempty"` // which classifier produced it: "gosqlx" | "heuristic"
}
