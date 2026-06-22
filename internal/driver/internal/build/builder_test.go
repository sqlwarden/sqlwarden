package build

import (
	"testing"

	"github.com/sqlwarden/internal/schema"
)

func group(t *testing.T, s *schema.Schema, ns, kind string) schema.ObjectGroup {
	t.Helper()
	for _, n := range s.Namespaces {
		if n.Name != ns {
			continue
		}
		for _, g := range n.ObjectGroups {
			if g.Kind == kind {
				return g
			}
		}
	}
	t.Fatalf("group %s/%s not found in %+v", ns, kind, s)
	return schema.ObjectGroup{}
}

func TestBuilderTablesAndViews(t *testing.T) {
	b := New()
	b.AddColumn("public", "users", false, schema.Column{Name: "id"})
	b.AddPrimaryKeyColumn("public", "users", "id")
	b.AddIndex("public", "users", schema.Index{Name: "users_pkey", Unique: true})
	b.AddColumn("public", "active_users", true, schema.Column{Name: "id"})

	s := b.Build("app")
	tables := group(t, s, "public", KindTable)
	if tables.Label != "Tables" || len(tables.Objects) != 1 || tables.Objects[0].Name != "users" {
		t.Fatalf("unexpected tables group: %+v", tables)
	}
	if len(tables.Objects[0].PrimaryKey) != 1 || len(tables.Objects[0].Indexes) != 1 {
		t.Fatalf("pk/index not attached: %+v", tables.Objects[0])
	}
	views := group(t, s, "public", KindView)
	if views.Label != "Views" || len(views.Objects) != 1 {
		t.Fatalf("unexpected views group: %+v", views)
	}
}

func TestBuilderDeclaredGroupOrderAndAttributes(t *testing.T) {
	b := New()
	b.DeclareGroup("function", "Functions")
	b.DeclareGroup("sequence", "Sequences")
	// add out of declaration order; emit order must follow DeclareGroup order.
	b.AddObject("public", "sequence", "users_id_seq")
	b.SetObjectAttribute("public", "sequence", "users_id_seq", "increment", 1)
	b.AddObject("public", "function", "now2")
	b.AddColumn("public", "users", false, schema.Column{Name: "id"})

	s := b.Build("app")
	kinds := []string{}
	for _, g := range s.Namespaces[0].ObjectGroups {
		kinds = append(kinds, g.Kind)
	}
	want := []string{"table", "function", "sequence"} // table pre-declared first; view empty, dropped
	if len(kinds) != len(want) {
		t.Fatalf("group order = %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("group order = %v, want %v", kinds, want)
		}
	}
	seq := group(t, s, "public", "sequence")
	if seq.Label != "Sequences" || seq.Objects[0].Attributes["increment"] != 1 {
		t.Fatalf("sequence attribute not preserved: %+v", seq)
	}
}

func TestBuilderUnknownKindEmittedLast(t *testing.T) {
	b := New()
	b.AddColumn("public", "users", false, schema.Column{Name: "id"})
	b.AddObject("public", "widget", "thingy") // never DeclareGroup'd

	s := b.Build("app")
	groups := s.Namespaces[0].ObjectGroups
	last := groups[len(groups)-1]
	if last.Kind != "widget" || last.Label != "widget" {
		t.Fatalf("undeclared kind should be last with kind as label, got %+v", last)
	}
}
