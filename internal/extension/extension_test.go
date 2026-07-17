package extension

import (
	"io/fs"
	"testing"
	"testing/fstest"
)

type bareExt struct{ name string }

func (e bareExt) Name() string { return e.name }

type migExt struct{ bareExt }

func (migExt) Migrations(string) (fs.FS, bool) { return fstest.MapFS{}, true }

func TestRegistryAddAndAll(t *testing.T) {
	r := NewRegistry()
	if got := r.All(); len(got) != 0 {
		t.Fatalf("new registry not empty: %d", len(got))
	}
	r.Add(bareExt{name: "a"}, migExt{bareExt{name: "b"}})
	all := r.All()
	if len(all) != 2 || all[0].Name() != "a" || all[1].Name() != "b" {
		t.Fatalf("unexpected registry contents: %+v", all)
	}
}

func TestCapabilityDiscoveryByTypeAssertion(t *testing.T) {
	r := NewRegistry()
	r.Add(bareExt{name: "a"}, migExt{bareExt{name: "b"}})

	var sources []string
	for _, ext := range r.All() {
		if _, ok := ext.(MigrationSource); ok {
			sources = append(sources, ext.Name())
		}
	}
	if len(sources) != 1 || sources[0] != "b" {
		t.Fatalf("expected only b to be a MigrationSource, got %v", sources)
	}
}
