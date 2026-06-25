package rewriter

import "context"

type Purpose string

const PurposePagination Purpose = "pagination"

// Rewriter transforms SQL when safe for a dialect-specific purpose. Stateless.
type Rewriter interface {
	Rewrite(ctx context.Context, req Request) (Result, error)
}

type Request struct {
	SQL     string
	Purpose Purpose
	Limit   int
	Offset  int
}

type Result struct {
	SQL     string `json:"sql"`
	Applied bool   `json:"applied"`
	Reason  string `json:"reason,omitempty"`
}
