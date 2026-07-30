// Package completer defines the SQL completion capability: cursor-aware
// suggestions for an in-progress statement. An engine provides completion by
// implementing Completer; it is stateless and never touches a live connection —
// any schema context it needs is passed in as a catalog by the caller.
package completer

import (
	"context"

	"github.com/sqlwarden/internal/dbengine/metadata"
)

// Completer returns suggestions for the text at a cursor position, optionally
// informed by a schema catalog. Stateless: the caller supplies the catalog
// rather than the completer fetching it, so completion needs no connection.
type Completer interface {
	Complete(ctx context.Context, req Request) (Result, error)
}

// VocabularyProvider exposes connection-independent lexical completions for a
// dialect. Vocabulary never contains customer schema metadata.
type VocabularyProvider interface {
	CompletionVocabulary() Vocabulary
}

// CatalogInvalidator is implemented by completers that cache prepared native
// catalogs or schema indexes. Connection lifecycle and schema refresh paths
// call it so stale or compliance-sensitive metadata is not retained.
type CatalogInvalidator interface {
	InvalidateCompletionCatalog(connectionID string)
}

// Request is the editor state to complete: the SQL, the cursor offset into it,
// and optional schema metadata for name-aware suggestions.
type Request struct {
	SQL          string
	CursorOffset int
	Schema       *metadata.MetadataSet
	ConnectionID string
	TriggerKind  TriggerKind
	TriggerChar  string
}

type TriggerKind string

const (
	TriggerInvoked   TriggerKind = "invoked"
	TriggerAutomatic TriggerKind = "automatic"
)

// Result is the ranked list of suggestions for the cursor position.
type Result struct {
	Suggestions []Suggestion `json:"suggestions"`
}

// Suggestion is a single completion candidate. ReplaceStart/ReplaceEnd delimit
// the span the editor should replace with InsertText; Score orders candidates.
type Suggestion struct {
	Label        string `json:"label"`
	DisplayLabel string `json:"display_label,omitempty"`
	Kind         string `json:"kind"`
	Detail       string `json:"detail,omitempty"`
	InsertText   string `json:"insert_text,omitempty"`
	ReplaceStart int    `json:"replace_start"`
	ReplaceEnd   int    `json:"replace_end"`
	Score        int    `json:"score,omitempty"`
}

// Vocabulary is immutable for a SQLWarden build. Version is a deterministic
// content hash so clients can identify identical payloads across engines.
type Vocabulary struct {
	Dialect     string       `json:"dialect"`
	Version     string       `json:"version"`
	Suggestions []Suggestion `json:"suggestions"`
}
