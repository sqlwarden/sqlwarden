package postgres

import (
	"context"
	metadata "github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/metadata/metadatatest"
	"slices"
	"testing"

	"github.com/sqlwarden/internal/engine/completioncore"
	"github.com/sqlwarden/internal/engine/completioncore/completiontest"
)

func hasCandidate(candidates []completioncore.Candidate, text string, kind completioncore.CandidateType) bool {
	for _, c := range candidates {
		if c.Text == text && c.Type == kind {
			return true
		}
	}
	return false
}

func TestCompleteResolvesRelationsWithoutNativeCatalog(t *testing.T) {
	metadata := completiontest.Metadata("postgres", "app", "public")
	candidates, _, err := Complete(context.Background(), "SELECT * FROM ", len("SELECT * FROM "), metadata)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCandidate(candidates, "inventory", completioncore.CandidateTable) {
		t.Fatalf("missing table candidate: %+v", candidates)
	}
	if !hasCandidate(candidates, "public", completioncore.CandidateSchema) {
		t.Fatalf("missing schema candidate: %+v", candidates)
	}
}

func TestCompleteResolvesQualifiedRelationsWithoutNativeCatalog(t *testing.T) {
	metadata := completiontest.Metadata("postgres", "app", "public")
	sql := "SELECT * FROM public.fi"
	candidates, _, err := Complete(context.Background(), sql, len(sql), metadata)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCandidate(candidates, "film", completioncore.CandidateTable) || hasCandidate(candidates, "public", completioncore.CandidateSchema) {
		t.Fatalf("qualified candidates = %+v", candidates)
	}
}

func TestCompleteResolvesBuiltinFunctionsWithoutNativeCatalog(t *testing.T) {
	sql := "SELECT lowe"
	candidates, _, err := Complete(context.Background(), sql, len(sql), completiontest.Metadata("postgres", "app", "public"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasCandidate(candidates, "lower", completioncore.CandidateFunction) {
		t.Fatalf("missing builtin function: %+v", candidates)
	}
}

func TestCompleteKeepsRelationsOutOfUnqualifiedProjection(t *testing.T) {
	metadata := completiontest.Metadata("postgres", "app", "public")
	for _, sql := range []string{"SELECT ", "SELECT lowe"} {
		candidates, _, err := Complete(context.Background(), sql, len(sql), metadata)
		if err != nil {
			t.Fatal(err)
		}
		if hasCandidate(candidates, "inventory", completioncore.CandidateTable) || hasCandidate(candidates, "public", completioncore.CandidateSchema) {
			t.Fatalf("%q leaked relation candidates: %+v", sql, candidates)
		}
	}
}

func TestCompleteResolvesRelationsInStatementTargetSlots(t *testing.T) {
	metadata := completiontest.Metadata("postgres", "app", "public")
	for _, sql := range []string{"INSERT INTO ", "INSERT INTO inv", "UPDATE ", "DELETE FROM ", "DROP TABLE ", "COMMENT ON TABLE "} {
		candidates, _, err := Complete(context.Background(), sql, len(sql), metadata)
		if err != nil {
			t.Fatal(err)
		}
		if !hasCandidate(candidates, "inventory", completioncore.CandidateTable) {
			t.Fatalf("%q missing table candidate: %+v", sql, candidates)
		}
	}
}

func TestCompleteKeepsRelationsOutOfDMLColumnSlots(t *testing.T) {
	metadata := completiontest.Metadata("postgres", "app", "public")
	for _, sql := range []string{"UPDATE inventory SET ", "INSERT INTO inventory ("} {
		candidates, _, err := Complete(context.Background(), sql, len(sql), metadata)
		if err != nil {
			t.Fatal(err)
		}
		if hasCandidate(candidates, "inventory", completioncore.CandidateTable) {
			t.Fatalf("%q leaked relation candidates: %+v", sql, candidates)
		}
	}
}

func TestCompleteQualifiesRelationsWithoutDefaultScope(t *testing.T) {
	app := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
	sales := app.Child(metadata.ScopeSegment{Kind: "schema", Name: "sales"})
	view := metadatatest.Build(metadatatest.Tree(true), metadatatest.Fixture{
		Refs: []metadata.ObjectRef{{Scope: sales, Kind: "table", Name: "orders"}},
	})
	resolver := completioncore.NewSchemaResolver(view, "")
	sql := "SELECT * FROM "
	candidates, _, err := Complete(context.Background(), sql, len(sql), resolver)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range candidates {
		if c.Text == "orders" {
			if !slices.Equal(c.Qualifier, []string{"sales"}) {
				t.Fatalf("qualifier = %v", c.Qualifier)
			}
			return
		}
	}
	t.Fatalf("orders missing: %+v", candidates)
}

func TestCompleteKeepsSameNamedRelationsInDifferentSchemas(t *testing.T) {
	app := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
	sales := app.Child(metadata.ScopeSegment{Kind: "schema", Name: "sales"})
	hr := app.Child(metadata.ScopeSegment{Kind: "schema", Name: "hr"})
	view := metadatatest.Build(metadatatest.Tree(true), metadatatest.Fixture{
		Refs: []metadata.ObjectRef{{Scope: sales, Kind: "table", Name: "orders"}, {Scope: hr, Kind: "table", Name: "orders"}},
	})
	sql := "SELECT * FROM "
	candidates, _, err := Complete(context.Background(), sql, len(sql), completioncore.NewSchemaResolver(view, ""))
	if err != nil {
		t.Fatal(err)
	}
	var qualifiers []string
	for _, c := range candidates {
		if c.Text == "orders" && len(c.Qualifier) == 1 {
			qualifiers = append(qualifiers, c.Qualifier[0])
		}
	}
	slices.Sort(qualifiers)
	if !slices.Equal(qualifiers, []string{"hr", "sales"}) {
		t.Fatalf("qualifiers = %v", qualifiers)
	}
}
