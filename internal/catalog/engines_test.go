package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/metadata"

	_ "github.com/sqlwarden/internal/engine/engines/postgres"
)

func TestEngineToViewReportsShowAllDatabases(t *testing.T) {
	tree := metadata.Tree{Nodes: map[string]metadata.Node{
		"database": {ShowAllDatabases: true},
	}}
	view := engineToView(engine.CapabilitySet{Tree: &tree})
	if !view.ShowAllDatabases {
		t.Fatal("navigator database node should enable show_all_databases")
	}
}

func TestEngineCatalogRequiresAccount(t *testing.T) {
	ctx := context.Background()
	service := New(Deps{})
	for name, p := range map[string]access.Principal{
		"anonymous": {},
		"team":      {Subject: access.SubjectRef{Kind: access.SubjectTeam, ID: 7}},
	} {
		if _, err := service.Engines(ctx, p); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s engines err = %v, want ErrForbidden", name, err)
		}
		if _, err := service.Engine(ctx, p, "postgres"); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s engine err = %v, want ErrForbidden", name, err)
		}
		if _, err := service.EngineConnectionFields(ctx, p, "postgres"); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s connection fields err = %v, want ErrForbidden", name, err)
		}
	}
}

func TestEngineCatalog(t *testing.T) {
	ctx := context.Background()
	service := New(Deps{})
	p := access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: 7}}

	list, err := service.Engines(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, view := range list.Engines {
		if view.ID == "postgres" {
			found = true
		}
	}
	if !found {
		t.Fatalf("engines = %+v, want postgres listed", list.Engines)
	}

	view, err := service.Engine(ctx, p, "postgres")
	if err != nil || view.ID != "postgres" || view.Dialect != "postgres" {
		t.Fatalf("engine = %+v err = %v", view, err)
	}
	if _, err := service.Engine(ctx, p, "does-not-exist"); !errors.Is(err, ErrUnknownEngine) {
		t.Fatalf("unknown engine err = %v, want ErrUnknownEngine", err)
	}

	fields, err := service.EngineConnectionFields(ctx, p, "postgres")
	if err != nil || len(fields.Fields) == 0 {
		t.Fatalf("fields = %+v err = %v, want postgres connection fields", fields, err)
	}
	if _, err := service.EngineConnectionFields(ctx, p, "does-not-exist"); !errors.Is(err, ErrUnknownEngine) {
		t.Fatalf("unknown driver err = %v, want ErrUnknownEngine", err)
	}
}
