package oracle

import (
	"reflect"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

func TestNavigatorTreeValid(t *testing.T) {
	if err := navigatorTree.Validate(); err != nil {
		t.Fatal(err)
	}
	if kind := navigatorTree.DatabaseKind(); kind != "schema" {
		t.Fatalf("DatabaseKind() = %q, want schema", kind)
	}
}

func TestNavigatorSchemaListsDatabaseLinks(t *testing.T) {
	links, ok := navigatorTree.Folder("schema", "db_links")
	if !ok {
		t.Fatal("schema has no db_links folder")
	}
	if links.Child != "db_link" || links.Label != "Database Links" {
		t.Fatalf("db_links folder = %+v", links)
	}
	node, ok := navigatorTree.Node("db_link")
	if !ok || !node.Leaf || !node.HasDefinition || node.Icon != "db_link" {
		t.Fatalf("db_link node = %+v, %v", node, ok)
	}
}

func TestMergeDependencies(t *testing.T) {
	dep := func(name, direction string) metadata.Child {
		return metadata.Child{Kind: "dependency", Name: name, Attributes: map[string]any{"direction": direction}}
	}
	got := mergeDependencies([]metadata.Child{
		dep("S.F", "dependency"), dep("S.F", "dependent"), dep("S.T", "dependency"), dep("S.T", "dependency"), dep("S.V", "dependent"),
	})
	want := []metadata.Child{dep("S.F", "both"), dep("S.T", "dependency"), dep("S.V", "dependent")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mergeDependencies = %+v, want %+v", got, want)
	}
}
