package sqlquery

import (
	"context"

	"github.com/sqlwarden/internal/schema"
)

// Completer returns cursor-aware suggestions using parse context and optional
// schema metadata.
type Completer interface {
	Complete(ctx context.Context, req CompletionRequest) (CompletionResult, error)
}

type CompletionRequest struct {
	RequestMetadata
	SQL          string
	CursorOffset int
	Catalog      *schema.Catalog
}

type CompletionResult struct {
	Suggestions []Suggestion `json:"suggestions"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type Suggestion struct {
	Label        string `json:"label"`
	Kind         string `json:"kind"`
	Detail       string `json:"detail,omitempty"`
	InsertText   string `json:"insert_text,omitempty"`
	ReplaceStart int    `json:"replace_start,omitempty"`
	ReplaceEnd   int    `json:"replace_end,omitempty"`
	Score        int    `json:"score,omitempty"`
}
