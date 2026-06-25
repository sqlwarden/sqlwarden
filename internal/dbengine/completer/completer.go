// Package completer defines the SQL completion capability: cursor-aware
// suggestions for an in-progress statement. An engine provides completion by
// implementing Completer; it is stateless and never touches a live connection —
// any schema context it needs is passed in as a catalog by the caller.
package completer

import (
	"context"

	"github.com/sqlwarden/internal/dbengine/schema"
)

// Completer returns suggestions for the text at a cursor position, optionally
// informed by a schema catalog. Stateless: the caller supplies the catalog
// rather than the completer fetching it, so completion needs no connection.
type Completer interface {
	Complete(ctx context.Context, req Request) (Result, error)
}

// Request is the editor state to complete: the SQL, the cursor offset into it,
// and optional schema metadata for name-aware suggestions.
type Request struct {
	SQL          string
	CursorOffset int
	Catalog      *schema.Catalog
}

// Result is the ranked list of suggestions for the cursor position.
type Result struct {
	Suggestions []Suggestion `json:"suggestions"`
}

// Suggestion is a single completion candidate. ReplaceStart/ReplaceEnd delimit
// the span the editor should replace with InsertText; Score orders candidates.
type Suggestion struct {
	Label        string `json:"label"`
	Kind         string `json:"kind"`
	Detail       string `json:"detail,omitempty"`
	InsertText   string `json:"insert_text,omitempty"`
	ReplaceStart int    `json:"replace_start,omitempty"`
	ReplaceEnd   int    `json:"replace_end,omitempty"`
	Score        int    `json:"score,omitempty"`
}
