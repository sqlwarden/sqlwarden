package postgres

import (
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

func TestSourceDescriptor(t *testing.T) {
	if got := SourceDescriptor("DDL", "sql", ""); got != nil {
		t.Fatalf("expected nil descriptor for empty body, got %+v", got)
	}
	got := SourceDescriptor("DDL", "sql", "CREATE TABLE t (id int);")
	if got == nil {
		t.Fatal("expected non-nil descriptor")
	}
	if got.Kind != "source" || got.Title != "DDL" {
		t.Fatalf("unexpected descriptor shape: %+v", got)
	}
	if got.Source == nil || got.Source.Language != "sql" || got.Source.Body != "CREATE TABLE t (id int);" {
		t.Fatalf("unexpected descriptor source: %+v", got.Source)
	}
}

func TestPostgresRequestedRefFallback(t *testing.T) {
	refs := []metadata.ObjectRef{
		{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "public"}), Kind: "table", Name: "users"},
	}
	got := postgresRequestedRef(refs, "public", "users", "table")
	if got.Name != "users" || got.Kind != "table" {
		t.Fatalf("expected exact match returned, got %+v", got)
	}
	got = postgresRequestedRef(refs, "public", "missing", "table")
	if got.Name != "missing" {
		t.Fatalf("expected fallback ref for unmatched name, got %+v", got)
	}
}
