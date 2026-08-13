package metadata

import (
	"reflect"
	"testing"
)

func TestIndexObjectLookupAndIsolation(t *testing.T) {
	scope := NewScopePath(ScopeSegment{Kind: "database", Name: "app"}, ScopeSegment{Kind: "schema", Name: "public"})
	directory := &Directory{
		Engine: "postgres", DefaultScope: scope,
		Roots: []ScopeNode{{
			Path: scope,
			Groups: []ObjectGroup{{
				Kind: "table",
				Objects: []ObjectRef{
					{Scope: scope, Kind: "table", Name: "Accounts"},
					{Scope: scope, Kind: "table", Name: "PendingDetails"},
				},
			}},
		}},
	}
	objects := []Object{{
		Ref: ObjectRef{Scope: scope, Kind: "table", Name: "Accounts"},
		Relational: &RelationalDetail{
			Columns: []Column{{Name: "id", DataType: "bigint", Attributes: map[string]any{"comment": "key"}}},
			Indexes: []SecondaryIndex{{Name: "accounts_pkey", Columns: []string{"id"}, Unique: true}},
		},
	}}
	index := NewIndex(MetadataSet{Directory: directory, Objects: objects, Version: "snapshot-7"})

	// Mutating constructor inputs must not mutate the prepared index.
	directory.Engine = "mutated"
	objects[0].Relational.Columns[0].Name = "mutated"
	if index.DefaultScope() != scope || index.Version() != "snapshot-7" {
		t.Fatalf("unexpected index identity: scope=%q version=%q", index.DefaultScope(), index.Version())
	}
	if ref, ok := index.FindRefInScope(scope, "table", "pendingdetails"); !ok || ref.Name != "PendingDetails" {
		t.Fatalf("directory-only ref lookup = %+v, %v", ref, ok)
	}
	if _, ok := index.FindObjectInScope(scope, "table", "PendingDetails"); ok {
		t.Fatal("directory-only ref unexpectedly had object detail")
	}
	object, ok := index.FindObjectInScope(scope, "TABLE", "accounts")
	if !ok || object.Ref.Name != "Accounts" || object.Relational.Columns[0].Name != "id" {
		t.Fatalf("case-folded object lookup = %+v, %v", object, ok)
	}

	// Mutating returned values must not mutate subsequent reads.
	object.Relational.Columns[0].Name = "changed"
	object.Relational.Indexes[0].Columns[0] = "changed"
	again, ok := index.Object(ObjectRef{Scope: scope, Kind: "table", Name: "Accounts"})
	if !ok || again.Relational.Columns[0].Name != "id" || again.Relational.Indexes[0].Columns[0] != "id" {
		t.Fatalf("index leaked mutable state: %+v", again)
	}
}

func TestIndexDirectoryLookupSupportsNonSQLScopes(t *testing.T) {
	scope := NewScopePath(
		ScopeSegment{Kind: "cluster", Name: "cache-prod"},
		ScopeSegment{Kind: "logical_database", Name: "2"},
	)
	ref := ObjectRef{Scope: scope, Kind: "key", Name: "session:42"}
	directory := &Directory{
		Engine:       "redis",
		DefaultScope: scope,
		Roots: []ScopeNode{{
			Path: NewScopePath(ScopeSegment{Kind: "cluster", Name: "cache-prod"}),
			Children: []ScopeNode{{
				Path: scope,
				Groups: []ObjectGroup{{
					Kind: "key", Objects: []ObjectRef{ref},
				}},
			}},
		}},
	}
	index := NewIndex(MetadataSet{
		Directory: directory,
		Objects:   []Object{{Ref: ref, Attributes: map[string]any{"type": "hash"}}},
		Version:   "ephemeral-1",
	})

	if index.DefaultScope() != scope {
		t.Fatalf("default scope = %q, want %q", index.DefaultScope(), scope)
	}
	if got, ok := index.FindRefInScope(scope, "key", "SESSION:42"); !ok || got != ref {
		t.Fatalf("FindRefInScope = %+v, %v", got, ok)
	}
	if got, ok := index.FindObjectInScope(scope, "key", "session:42"); !ok || got.Ref != ref {
		t.Fatalf("FindObjectInScope = %+v, %v", got, ok)
	}
}

func TestIndexRelationshipGraphLookups(t *testing.T) {
	scope := NewScopePath(ScopeSegment{Kind: "schema", Name: "public"})
	users := ObjectRef{Scope: scope, Kind: "table", Name: "users"}
	orders := ObjectRef{Scope: scope, Kind: "table", Name: "orders"}
	items := ObjectRef{Scope: scope, Kind: "table", Name: "order_items"}
	objects := []Object{{Ref: orders}}
	graph := &RelationshipGraph{
		Scope: scope,
		Relationships: []Relationship{
			{Name: "orders_user_fk", Source: orders, Columns: []string{"user_id"}, References: users, ReferencedColumns: []string{"id"}},
			{Name: "items_order_fk", Source: items, Columns: []string{"order_id"}, References: orders, ReferencedColumns: []string{"id"}},
		},
	}
	index := NewIndex(MetadataSet{Objects: objects, Relationships: []*RelationshipGraph{graph}})

	if got := index.Outgoing(orders); len(got) != 1 || got[0].References != users {
		t.Fatalf("outgoing orders = %+v", got)
	}
	if got := index.Incoming(orders); len(got) != 1 || got[0].Source != items {
		t.Fatalf("incoming orders = %+v", got)
	}
	gotRefs := index.NeighborRefs(orders)
	wantRefs := []ObjectRef{items, users}
	if !reflect.DeepEqual(gotRefs, wantRefs) {
		t.Fatalf("neighbors = %+v, want %+v", gotRefs, wantRefs)
	}
	if got := index.Neighbors(orders); len(got) != 0 {
		t.Fatalf("neighbors with uninspected details = %+v", got)
	}

	relationships := index.RelationshipsInScope(scope)
	relationships[0].Columns[0] = "mutated"
	if got := index.RelationshipsInScope(scope); got[0].Columns[0] != "user_id" {
		t.Fatalf("relationships leaked mutable state: %+v", got)
	}
}
