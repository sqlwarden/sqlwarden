package sqlquery

import (
	"errors"

	"github.com/sqlwarden/internal/driver"
)

// ErrUnsupportedCapability is returned when a dialect provider does not yet
// support a parser, completer, or rewriter capability.
var ErrUnsupportedCapability = errors.New("sqlquery: unsupported capability")

// Kind is the coarse statement class used for RBAC query execution checks.
type Kind string

const (
	KindUnknown Kind = "unknown"
	KindDQL     Kind = "dql"
	KindDML     Kind = "dml"
	KindDDL     Kind = "ddl"
)

// Diagnostic describes a non-fatal parser or analyzer issue.
type Diagnostic struct {
	Message  string `json:"message"`
	Severity string `json:"severity,omitempty"`
	Offset   *int   `json:"offset,omitempty"`
}

// RequestMetadata carries common query-analysis context.
type RequestMetadata struct {
	Dialect driver.Dialect
}
