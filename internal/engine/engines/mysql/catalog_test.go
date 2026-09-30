package mysql

import (
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

func TestMySQLSourceDescriptor(t *testing.T) {
	if got := SourceDescriptor("DDL", ""); got != nil {
		t.Fatalf("expected nil descriptor for empty body, got %+v", got)
	}
	got := SourceDescriptor("DDL", "CREATE TABLE t (id int);")
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

func TestMySQLQuoteQualified(t *testing.T) {
	got := mysqlQuoteQualified("my`db", "my`table")
	want := "`my``db`.`my``table`"
	if got != want {
		t.Fatalf("mysqlQuoteQualified() = %q, want %q", got, want)
	}
}

func TestMySQLRequestedRefFallback(t *testing.T) {
	refs := []metadata.ObjectRef{
		{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"}), Kind: "table", Name: "users"},
	}
	got := mysqlRequestedRef(refs, "app", "users", "table")
	if got.Name != "users" {
		t.Fatalf("expected exact match, got %+v", got)
	}
	got = mysqlRequestedRef(refs, "app", "missing", "table")
	if got.Name != "missing" || got.Scope.Name("database") != "app" {
		t.Fatalf("expected fallback ref, got %+v", got)
	}
}
