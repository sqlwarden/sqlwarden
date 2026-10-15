package schema

import (
	"context"
	"slices"
	"testing"

	metadata "github.com/sqlwarden/internal/engine/metadata"
)

func TestDiscoverScopesFromRootDescendsIntoCurrentScopes(t *testing.T) {
	cat := newFakeCatalog()
	cat.set("", "databases",
		metadata.Child{Kind: "database", Name: "app", Current: true},
		metadata.Child{Kind: "database", Name: "reports"})
	cat.set(dbPath("app"), "schemas",
		metadata.Child{Kind: "schema", Name: "pg_catalog", System: true},
		metadata.Child{Kind: "schema", Name: "public", Current: true},
		metadata.Child{Kind: "schema", Name: "sales"})

	got, err := DiscoverScopes(context.Background(), cat.Tree(), cat, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Current != schemaPathOf("app", "public") {
		t.Fatalf("current = %q", got.Current)
	}
	want := []metadata.ScopePath{dbPath("app"), dbPath("reports"), schemaPathOf("app", "public"), schemaPathOf("app", "sales")}
	if !slices.Equal(got.Scopes, want) {
		t.Fatalf("scopes = %q, want %q", got.Scopes, want)
	}
	if !slices.Equal(cat.callLog(), []string{"databases:1", "schemas:1"}) {
		t.Fatalf("calls = %v", cat.callLog())
	}
	if !slices.Equal(cat.databases, []string{"", "app"}) {
		t.Fatalf("queriers = %v", cat.databases)
	}
}

func TestDiscoverScopesUnderParentListsOnlyItsScopeChildren(t *testing.T) {
	cat := newFakeCatalog()
	cat.set(dbPath("reports"), "schemas", metadata.Child{Kind: "schema", Name: "monthly"})

	got, err := DiscoverScopes(context.Background(), cat.Tree(), cat, dbPath("reports"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Current != dbPath("reports") {
		t.Fatalf("current = %q", got.Current)
	}
	if !slices.Equal(got.Scopes, []metadata.ScopePath{schemaPathOf("reports", "monthly")}) {
		t.Fatalf("scopes = %q", got.Scopes)
	}
	if !slices.Equal(cat.callLog(), []string{"schemas:1"}) {
		t.Fatalf("calls = %v", cat.callLog())
	}
}

func TestDiscoverScopesWithoutCurrentStopsAtFirstLevel(t *testing.T) {
	cat := newFakeCatalog()
	cat.set("", "databases", metadata.Child{Kind: "database", Name: "app"})

	got, err := DiscoverScopes(context.Background(), cat.Tree(), cat, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Current != "" || !slices.Equal(got.Scopes, []metadata.ScopePath{dbPath("app")}) {
		t.Fatalf("got = %+v", got)
	}
	if !slices.Equal(cat.callLog(), []string{"databases:1"}) {
		t.Fatalf("calls = %v", cat.callLog())
	}
}

func TestDiscoverScopesUnknownParentKind(t *testing.T) {
	cat := newFakeCatalog()
	_, err := DiscoverScopes(context.Background(), cat.Tree(), cat, metadata.NewScopePath(seg("warehouse", "x")))
	if err == nil {
		t.Fatal("want error for a parent kind outside the grammar")
	}
}
