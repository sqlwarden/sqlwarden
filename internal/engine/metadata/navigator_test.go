package metadata

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func noopLoader(context.Context, Querier, []ScopePath) (map[ScopePath][]Child, error) {
	return map[ScopePath][]Child{}, nil
}

func sampleTree() Tree {
	return Tree{
		Root: Node{Label: "Connection", Icon: "connection", Folders: []Folder{
			{Kind: "databases", Label: "Databases", Child: "database", Order: 10, List: noopLoader},
		}},
		Nodes: map[string]Node{
			"database": {Label: "Database", Icon: "database", Scope: true, ShowAllDatabases: true, Folders: []Folder{
				{Kind: "schemas", Label: "Schemas", Child: "schema", Order: 10, List: noopLoader},
			}},
			"schema": {Label: "Schema", Icon: "schema", Scope: true, Folders: []Folder{
				{Kind: "tables", Label: "Tables", Child: "table", Order: 10, List: noopLoader},
				{Kind: "functions", Label: "Functions", Child: "function", MixedKinds: []string{"function", "procedure"}, Order: 20, List: noopLoader},
			}},
			"table":     {Label: "Table", Icon: "table", Relational: true, Folders: []Folder{{Kind: "columns", Label: "Columns", Child: "column", Order: 10, List: noopLoader}}},
			"column":    {Label: "Column", Icon: "column", Leaf: true, Column: true},
			"function":  {Label: "Function", Icon: "function", Leaf: true},
			"procedure": {Label: "Procedure", Icon: "procedure", Leaf: true},
		},
	}
}

func path(parts ...string) ScopePath {
	segments := make([]ScopeSegment, 0, len(parts)/2)
	for i := 0; i+1 < len(parts); i += 2 {
		segments = append(segments, ScopeSegment{Kind: parts[i], Name: parts[i+1]})
	}
	return NewScopePath(segments...)
}

func TestScopePathParentDepthPrefixes(t *testing.T) {
	p := path("database", "app", "schema", "public", "table", "users")
	if got := p.Parent(); got != path("database", "app", "schema", "public") {
		t.Fatalf("Parent = %q", got)
	}
	if p.Depth() != 3 || ScopePath("").Depth() != 0 {
		t.Fatalf("Depth = %d", p.Depth())
	}
	if ScopePath("").Parent() != "" {
		t.Fatal("root parent must be root")
	}
	want := []ScopePath{"", path("database", "app"), path("database", "app", "schema", "public")}
	got := p.Prefixes()
	if len(got) != len(want) {
		t.Fatalf("Prefixes = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Prefixes[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestScopePathWithinRespectsSegmentBoundary(t *testing.T) {
	a := path("database", "a")
	if path("database", "ab").Within(a) {
		t.Fatal("database=ab must not be within database=a")
	}
	if !path("database", "a", "schema", "s").Within(a) || !a.Within(a) || !a.Within("") {
		t.Fatal("expected within")
	}
	slash := path("database", "a/b")
	if strings.Count(string(slash), "/") != 0 {
		t.Fatalf("slash in name must be escaped: %q", slash)
	}
	if path("database", "a/b", "schema", "s").Within(a) {
		t.Fatal("escaped slash must not create a boundary")
	}
}

func TestObjectRefPathRoundTrip(t *testing.T) {
	p := path("database", "app", "schema", "public", "table", "users")
	ref, ok := ObjectRefOf(p)
	if !ok || ref.Kind != "table" || ref.Name != "users" || ref.Scope != path("database", "app", "schema", "public") {
		t.Fatalf("ObjectRefOf = %+v %v", ref, ok)
	}
	if ref.Path() != p {
		t.Fatalf("Path = %q", ref.Path())
	}
	if _, ok := ObjectRefOf(""); ok {
		t.Fatal("root has no object ref")
	}
}

func TestTreeLookups(t *testing.T) {
	tree := sampleTree()
	if root, ok := tree.Node(""); !ok || root.Label != "Connection" {
		t.Fatal("empty kind must resolve to root")
	}
	if f, ok := tree.Folder("schema", "tables"); !ok || f.Child != "table" {
		t.Fatal("Folder lookup failed")
	}
	if tree.DatabaseKind() != "database" {
		t.Fatalf("DatabaseKind = %q", tree.DatabaseKind())
	}
	p := path("database", "app", "schema", "public")
	if tree.DatabaseOf(p) != "app" || tree.DatabaseOf("") != "" {
		t.Fatal("DatabaseOf failed")
	}
	if tree.NodeKindOf(p) != "schema" || tree.NodeKindOf("") != "" {
		t.Fatal("NodeKindOf failed")
	}
	if f, ok := tree.FolderContaining("schema", "procedure"); !ok || f.Kind != "functions" {
		t.Fatal("FolderContaining must honor MixedKinds")
	}
}

func TestTreeValidate(t *testing.T) {
	if err := sampleTree().Validate(); err != nil {
		t.Fatalf("valid tree rejected: %v", err)
	}
	cases := map[string]func(Tree) Tree{
		"undeclared child": func(t Tree) Tree {
			return t.WithFolder("schema", Folder{Kind: "views", Label: "Views", Child: "view", List: noopLoader})
		},
		"undeclared mixed kind": func(t Tree) Tree {
			return t.WithFolder("schema", Folder{Kind: "functions", Label: "Functions", Child: "function", MixedKinds: []string{"aggregate"}, List: noopLoader})
		},
		"unknown icon": func(t Tree) Tree {
			n := t.Nodes["table"]
			n.Icon = "rocket"
			return t.WithNode("table", n)
		},
		"leaf with folders": func(t Tree) Tree {
			n := t.Nodes["column"]
			n.Folders = []Folder{{Kind: "x", Label: "X", Child: "column", List: noopLoader}}
			return t.WithNode("column", n)
		},
		"duplicate folder kind": func(t Tree) Tree {
			n := t.Nodes["table"]
			n.Folders = append(n.Folders, n.Folders[0])
			return t.WithNode("table", n)
		},
		"nil loader": func(t Tree) Tree {
			return t.WithFolder("table", Folder{Kind: "columns", Label: "Columns", Child: "column"})
		},
		"two database kinds": func(t Tree) Tree {
			n := t.Nodes["schema"]
			n.ShowAllDatabases = true
			return t.WithNode("schema", n)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if err := mutate(sampleTree()).Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestTreeDerivationDoesNotMutateSource(t *testing.T) {
	base := sampleTree()
	derived := base.WithoutFolder("schema", "functions")
	if _, ok := derived.Folder("schema", "functions"); ok {
		t.Fatal("WithoutFolder did not remove folder")
	}
	if _, ok := base.Folder("schema", "functions"); !ok {
		t.Fatal("WithoutFolder mutated the source tree")
	}
	replaced := base.WithFolder("schema", Folder{Kind: "tables", Label: "Base Tables", Child: "table", List: noopLoader})
	if f, _ := replaced.Folder("schema", "tables"); f.Label != "Base Tables" {
		t.Fatal("WithFolder must replace by kind")
	}
	if f, _ := base.Folder("schema", "tables"); f.Label != "Tables" {
		t.Fatal("WithFolder mutated the source tree")
	}
}

func TestTreeJSONOmitsLoadersAndSortsFolders(t *testing.T) {
	tree := sampleTree().WithFolder("schema", Folder{Kind: "early", Label: "Early", Child: "table", Order: 1, List: noopLoader})
	raw, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "List") || !strings.Contains(text, `"show_all_databases":true`) {
		t.Fatalf("unexpected JSON: %s", text)
	}
	var decoded struct {
		Nodes map[string]struct {
			Folders []struct {
				Kind string `json:"kind"`
			} `json:"folders"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Nodes["schema"].Folders[0].Kind != "early" {
		t.Fatalf("folders must serialize in Order: %s", text)
	}
}
