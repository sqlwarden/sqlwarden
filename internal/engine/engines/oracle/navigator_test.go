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
