package sqlquery

import "context"

// Rewriter transforms SQL when it can do so safely for a dialect-specific
// purpose such as server-side pagination.
type Rewriter interface {
	Rewrite(ctx context.Context, req RewriteRequest) (RewriteResult, error)
}

type RewritePurpose string

const (
	RewritePurposePagination RewritePurpose = "pagination"
)

type RewriteRequest struct {
	RequestMetadata
	SQL     string
	Purpose RewritePurpose
	Limit   int
	Offset  int
}

type RewriteResult struct {
	SQL         string       `json:"sql"`
	Applied     bool         `json:"applied"`
	Reason      string       `json:"reason,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}
