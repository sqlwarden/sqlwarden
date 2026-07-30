package classifier

import (
	"context"
	"strings"
)

type heuristic struct{}

// NewHeuristic returns the conservative, dialect-agnostic keyword classifier
// used as a fallback for dialects without a registered classifier.
func NewHeuristic() Classifier { return heuristic{} }

func (heuristic) Classify(_ context.Context, req Request) (Result, error) {
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
	return Result{Kind: kind, Source: "heuristic"}, nil
}
