package schema

import (
	"context"
	"errors"
	"slices"
	"testing"

	metadata "github.com/sqlwarden/internal/engine/metadata"
)

type fakeRelationships struct {
	calls int
	graph *metadata.RelationshipGraph
}

func (f *fakeRelationships) InspectRelationshipsInScope(_ context.Context, scope metadata.ScopePath) (*metadata.RelationshipGraph, error) {
	f.calls++
	return &metadata.RelationshipGraph{Scope: scope, Relationships: f.graph.Relationships}, nil
}

func TestObjectsInspectsOnlyMisses(t *testing.T) {
	cat := newFakeCatalog()
	users := metadata.ObjectRef{Scope: schemaPathOf("app", "public"), Kind: "table", Name: "users"}
	orders := metadata.ObjectRef{Scope: schemaPathOf("app", "public"), Kind: "table", Name: "orders"}
	missing := metadata.ObjectRef{Scope: schemaPathOf("app", "public"), Kind: "table", Name: "gone"}
	cat.objects[users] = metadata.Object{Ref: users, Attributes: map[string]any{"n": "u"}}
	cat.objects[orders] = metadata.Object{Ref: orders}
	store := newFakeStore()
	n := newTestNavigator(store)
	conn := Connection{ID: 1, Persistent: true}
	ctx := context.Background()

	if _, err := n.Objects(ctx, conn, cat, []metadata.ObjectRef{users}); err != nil {
		t.Fatal(err)
	}
	got, err := n.Objects(ctx, conn, cat, []metadata.ObjectRef{orders, users, missing})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Ref != orders || got[1].Ref != users {
		t.Fatalf("got = %+v", got)
	}
	if calls := cat.callLog(); !slices.Equal(calls, []string{"objects:1", "objects:2"}) {
		t.Fatalf("calls = %v", calls)
	}
	restarted := newTestNavigator(store)
	fromStore, err := restarted.Objects(ctx, conn, nil, []metadata.ObjectRef{users})
	if err != nil || len(fromStore) != 1 || fromStore[0].Attributes["n"] != "u" {
		t.Fatalf("fromStore = %+v, %v", fromStore, err)
	}
	if _, err := restarted.Objects(ctx, conn, nil, []metadata.ObjectRef{missing}); !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("err = %v", err)
	}
}

func TestRelationshipsLookupChainAndInvalidate(t *testing.T) {
	scope := schemaPathOf("app", "public")
	live := &fakeRelationships{graph: &metadata.RelationshipGraph{Relationships: []metadata.Relationship{{Name: "fk"}}}}
	store := newFakeStore()
	n := newTestNavigator(store)
	conn := Connection{ID: 1, Persistent: true}
	ctx := context.Background()

	if _, err := n.Relationships(ctx, conn, live, scope); err != nil {
		t.Fatal(err)
	}
	graph, err := newTestNavigator(store).Relationships(ctx, conn, nil, scope)
	if err != nil || len(graph.Relationships) != 1 || graph.Relationships[0].Name != "fk" {
		t.Fatalf("graph = %+v, %v", graph, err)
	}
	if err := n.Invalidate(ctx, conn, scope.Child(seg("table", "users"))); err != nil {
		t.Fatal(err)
	}
	if _, err := n.Relationships(ctx, conn, nil, scope); !errors.Is(err, ErrSessionRequired) {
		t.Fatalf("ancestor graph must be invalidated: %v", err)
	}
	if live.calls != 1 {
		t.Fatalf("live calls = %d", live.calls)
	}
}
