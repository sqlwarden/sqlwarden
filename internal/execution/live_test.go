package execution

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/internal/schema"
)

func TestLiveAdaptsLocalRuntime(t *testing.T) {
	r, info := fkSession(t)
	ctx := context.Background()
	caps, err := r.Capabilities(ctx, baseScope, info.ID)
	if err != nil || caps.Schema == nil {
		t.Fatalf("capabilities: %+v %v", caps, err)
	}
	tree := *caps.Schema
	var live schema.Live = NewLive(r, baseScope, info.ID, tree)

	folder, ok := tree.Folder(tree.NodeKindOf(mainDB), "tables")
	if !ok {
		t.Fatal("tables folder missing")
	}
	tables, err := live.LoadChildren(ctx, tree.DatabaseOf(mainDB), folder, []metadata.ScopePath{mainDB})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, c := range tables[mainDB] {
		names[c.Name] = true
	}
	if !names["parent"] || !names["child"] {
		t.Fatalf("tables: %+v", tables)
	}

	empty, err := live.LoadChildren(ctx, "", folder, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty batch = %+v, %v", empty, err)
	}

	ref := metadata.ObjectRef{Scope: mainDB, Kind: "table", Name: "parent"}
	objects, err := live.InspectObjects(ctx, []metadata.ObjectRef{ref})
	if err != nil || len(objects) != 1 || objects[0].Ref != ref {
		t.Fatalf("objects = %+v, %v", objects, err)
	}

	graph, err := live.InspectRelationships(ctx, mainDB)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Relationships) == 0 {
		t.Fatalf("relationships = %+v", graph)
	}

	if _, err := live.CurrentScope(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestLiveSurfacesClosedSession(t *testing.T) {
	r, info := fkSession(t)
	caps, err := r.Capabilities(context.Background(), baseScope, info.ID)
	if err != nil {
		t.Fatal(err)
	}
	live := NewLive(r, baseScope, "missing", *caps.Schema)
	if _, err := live.CurrentScope(context.Background()); err == nil {
		t.Fatal("want error for unknown session")
	}
}
