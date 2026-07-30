package completioncore

import (
	"context"
	"testing"
	"time"

	"github.com/sqlwarden/internal/engine/metadata"
)

func TestSchemaResolverAdaptsIndexMetadata(t *testing.T) {
	root := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"})
	public := root.Child(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	reporting := root.Child(metadata.ScopeSegment{Kind: "schema", Name: "reporting"})
	directory := &metadata.Directory{
		Engine: "postgres", DefaultScope: reporting,
		Roots: []metadata.ScopeNode{{Path: root, Children: []metadata.ScopeNode{{Path: public}, {Path: reporting}}}},
	}
	objects := []metadata.Object{{
		Ref: metadata.ObjectRef{Scope: reporting, Kind: "view", Name: "Daily Sales"},
		Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{
			Name: "Total", DataType: "numeric", Nullable: false,
			Attributes: map[string]any{"comment": "daily total"},
		}}},
		Descriptors: []metadata.Descriptor{{
			Kind: "source", Source: &metadata.Source{Language: "sql", Body: "SELECT 1 AS Total"},
		}},
	}}

	index := metadata.NewIndex(metadata.MetadataSet{Directory: directory, Objects: objects, Version: "v1"})
	resolver := NewSchemaResolver(index, "reporting")
	if resolver.DefaultDatabase() != "app" || resolver.DefaultSchema() != "reporting" {
		t.Fatalf("unexpected defaults: %q %q", resolver.DefaultDatabase(), resolver.DefaultSchema())
	}
	if got := resolver.DatabaseNames(); len(got) != 1 || got[0] != "app" {
		t.Fatalf("database names = %#v", got)
	}
	if got := resolver.SchemaNames(""); len(got) != 2 || got[0] != "public" || got[1] != "reporting" {
		t.Fatalf("schema names = %#v", got)
	}
	relation, ok := resolver.FindRelation("", "reporting", "Daily Sales")
	if !ok || relation.Kind != CandidateView || relation.Definition == "" {
		t.Fatalf("relation = %+v, %v", relation, ok)
	}
	if len(relation.Columns) != 1 || relation.Columns[0].Comment != "daily total" {
		t.Fatalf("columns = %+v", relation.Columns)
	}
	if got := ColumnDefinition(relation, relation.Columns[0]); got != "reporting.Daily Sales | numeric, NOT NULL" {
		t.Fatalf("definition = %q", got)
	}
}

func TestSchemaResolverDefaultAndMySQLNamespaceFallbacks(t *testing.T) {
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "sakila"})
	directory := &metadata.Directory{Engine: "mysql", DefaultScope: scope, Roots: []metadata.ScopeNode{{Path: scope}}}
	objects := []metadata.Object{{
		Ref: metadata.ObjectRef{Scope: scope, Kind: "table", Name: "film"},
		Relational: &metadata.RelationalDetail{Columns: []metadata.Column{{
			Name: "film_id", DataType: "smallint",
		}}},
	}}
	index := metadata.NewIndex(metadata.MetadataSet{Directory: directory, Objects: objects})
	resolver := NewSchemaResolver(index, "")
	if _, ok := resolver.FindRelation("", "", "film"); !ok {
		t.Fatal("MySQL database-as-namespace fallback did not resolve film")
	}
	if got := resolver.Relations("", "sakila"); len(got) != 1 || got[0].Name != "film" {
		t.Fatalf("relations = %+v", got)
	}
}

func TestCheckContext(t *testing.T) {
	if err := CheckContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := CheckContext(ctx); err == nil {
		t.Fatal("expected cancellation")
	}
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if err := CheckContext(ctx); err == nil {
		t.Fatal("expected deadline error")
	}
}
