package mysql

import (
	"context"
	"slices"
	"testing"

	"github.com/sqlwarden/internal/engine/completioncore"
	"github.com/sqlwarden/internal/engine/completioncore/completiontest"
	metadata "github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/engine/metadata/metadatatest"
)

func hasCandidate(candidates []completioncore.Candidate, text string, kind completioncore.CandidateType) bool {
	for _, candidate := range candidates {
		if candidate.Text == text && candidate.Type == kind {
			return true
		}
	}
	return false
}

func TestCompleteResolvesTablesWithoutNativeCatalog(t *testing.T) {
	sql := "SELECT * FROM "
	candidates, _, err := Complete(context.Background(), sql, len(sql), completiontest.Metadata("mysql", "sakila", "sakila"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasCandidate(candidates, "film", completioncore.CandidateTable) {
		t.Fatalf("missing table: %+v", candidates)
	}
}

func TestCompleteResolvesDatabaseQualifiedTablesWithoutNativeCatalog(t *testing.T) {
	sql := "SELECT * FROM sakila.fi"
	candidates, _, err := Complete(context.Background(), sql, len(sql), completiontest.Metadata("mysql", "sakila", "sakila"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasCandidate(candidates, "film", completioncore.CandidateTable) {
		t.Fatalf("missing qualified table: %+v", candidates)
	}
}

func TestCompleteResolvesDatabasesWithoutNativeCatalog(t *testing.T) {
	sql := "USE "
	candidates, _, err := Complete(context.Background(), sql, len(sql), completiontest.Metadata("mysql", "sakila", "sakila"))
	if err != nil {
		t.Fatal(err)
	}
	if !hasCandidate(candidates, "sakila", completioncore.CandidateDatabase) {
		t.Fatalf("missing database: %+v", candidates)
	}
}

func TestCompleteKeepsTablesOutOfUnqualifiedProjectionWithoutNativeCatalog(t *testing.T) {
	metadata := completiontest.Metadata("mysql", "sakila", "sakila")
	for _, sql := range []string{"SELECT ", "SELECT fi"} {
		candidates, _, err := Complete(context.Background(), sql, len(sql), metadata)
		if err != nil {
			t.Fatal(err)
		}
		if hasCandidate(candidates, "film", completioncore.CandidateTable) {
			t.Fatalf("%q leaked table candidates: %+v", sql, candidates)
		}
	}
}

func TestCompleteResolvesTablesQualifiedByNonDefaultDatabase(t *testing.T) {
	sakila := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "sakila"})
	world := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "world"})
	city := metadata.Object{Ref: metadata.ObjectRef{Scope: world, Kind: "table", Name: "city"}, Relational: &metadata.RelationalDetail{}}
	view := metadatatest.Build(metadatatest.Tree(false), metadatatest.Fixture{
		DefaultScope: sakila, Scopes: []metadata.ScopePath{sakila, world}, Objects: []metadata.Object{city},
	})
	resolver := completioncore.NewSchemaResolver(view, "sakila")
	sql := "SELECT * FROM world.ci"
	candidates, _, err := Complete(context.Background(), sql, len(sql), resolver)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCandidate(candidates, "city", completioncore.CandidateTable) {
		t.Fatalf("missing cross-database table: %+v", candidates)
	}
}

func TestCompleteResolvesTablesInStatementTargetSlots(t *testing.T) {
	metadata := completiontest.Metadata("mysql", "sakila", "sakila")
	for _, sql := range []string{"INSERT INTO ", "INSERT INTO fil", "UPDATE ", "DELETE FROM ", "DROP TABLE ", "TRUNCATE TABLE "} {
		candidates, _, err := Complete(context.Background(), sql, len(sql), metadata)
		if err != nil {
			t.Fatal(err)
		}
		if !hasCandidate(candidates, "film", completioncore.CandidateTable) {
			t.Fatalf("%q missing table candidate: %+v", sql, candidates)
		}
	}
}

func TestCompleteKeepsTablesOutOfDMLColumnSlots(t *testing.T) {
	metadata := completiontest.Metadata("mysql", "sakila", "sakila")
	for _, sql := range []string{"UPDATE film SET ", "UPDATE film SET title = 1 WHERE ", "INSERT INTO film (", "INSERT INTO film VALUES (", "DELETE FROM film WHERE ", "SELECT * FROM film WHERE "} {
		candidates, _, err := Complete(context.Background(), sql, len(sql), metadata)
		if err != nil {
			t.Fatal(err)
		}
		if hasCandidate(candidates, "film", completioncore.CandidateTable) || hasCandidate(candidates, "customer", completioncore.CandidateTable) {
			t.Fatalf("%q leaked table candidates: %+v", sql, candidates)
		}
	}
}

func TestCompleteQualifiesRelationsWithoutDefaultScope(t *testing.T) {
	shop := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "shop"})
	view := metadatatest.Build(metadatatest.Tree(false), metadatatest.Fixture{
		Refs: []metadata.ObjectRef{{Scope: shop, Kind: "table", Name: "orders"}},
	})
	resolver := completioncore.NewSchemaResolver(view, "")
	sql := "SELECT * FROM "
	candidates, _, err := Complete(context.Background(), sql, len(sql), resolver)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range candidates {
		if c.Text == "orders" {
			if !slices.Equal(c.Qualifier, []string{"shop"}) {
				t.Fatalf("qualifier = %v", c.Qualifier)
			}
			return
		}
	}
	t.Fatalf("orders missing: %+v", candidates)
}

func TestCompleteKeepsSameNamedRelationsInDifferentDatabases(t *testing.T) {
	shop := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "shop"})
	archive := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "archive"})
	view := metadatatest.Build(metadatatest.Tree(false), metadatatest.Fixture{
		Refs: []metadata.ObjectRef{{Scope: shop, Kind: "table", Name: "orders"}, {Scope: archive, Kind: "table", Name: "orders"}},
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
	if !slices.Equal(qualifiers, []string{"archive", "shop"}) {
		t.Fatalf("qualifiers = %v", qualifiers)
	}
}
