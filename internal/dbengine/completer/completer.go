package completer

import (
	"context"

	"github.com/sqlwarden/internal/dbengine/schema"
)

// Completer returns cursor-aware suggestions from parse context and optional
// schema metadata. Stateless: the caller passes the catalog in.
type Completer interface {
	Complete(ctx context.Context, req Request) (Result, error)
}

type Request struct {
	SQL          string
	CursorOffset int
	Catalog      *schema.Catalog
}

type Result struct {
	Suggestions []Suggestion `json:"suggestions"`
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
