package sqlquery

import (
	"context"
	"strings"
)

// Classifier determines the coarse class of SQL statements for authorization
// and execution routing.
type Classifier interface {
	Classify(ctx context.Context, req ClassifyRequest) (ClassifyResult, error)
}

type ClassifyRequest struct {
	RequestMetadata
	SQL string
}

type ClassifyResult struct {
	Kind        Kind         `json:"kind"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	Source      string       `json:"source,omitempty"`
}

type heuristicClassifier struct{}

// NewHeuristicClassifier returns the current lightweight classifier. It is
// intentionally conservative and preserves the legacy handler behavior until
// parser-backed implementations are introduced.
func NewHeuristicClassifier() Classifier {
	return heuristicClassifier{}
}

func (heuristicClassifier) Classify(_ context.Context, req ClassifyRequest) (ClassifyResult, error) {
	normalized := strings.ToUpper(strings.TrimSpace(req.SQL))
	kind := KindUnknown

	switch {
	case strings.Contains(normalized, "CREATE "), strings.Contains(normalized, "ALTER "), strings.Contains(normalized, "DROP "), strings.Contains(normalized, "TRUNCATE "), strings.Contains(normalized, "RENAME "):
		kind = KindDDL
	case strings.Contains(normalized, "INSERT "), strings.Contains(normalized, "UPDATE "), strings.Contains(normalized, "DELETE "), strings.Contains(normalized, "MERGE "), strings.Contains(normalized, "UPSERT "):
		kind = KindDML
	case strings.Contains(normalized, "SELECT "), strings.Contains(normalized, "SHOW "), strings.Contains(normalized, "DESCRIBE "), strings.Contains(normalized, "EXPLAIN "), strings.HasPrefix(normalized, "WITH "):
		kind = KindDQL
	}

	return ClassifyResult{Kind: kind, Source: "heuristic"}, nil
}
