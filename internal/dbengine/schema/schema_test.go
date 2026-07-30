package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestScopePathRoundTripAndJSON(t *testing.T) {
	path := NewScopePath(
		ScopeSegment{Kind: "database", Name: "sales/eu"},
		ScopeSegment{Kind: "schema", Name: `odd=name`},
	)
	segments, err := path.Segments()
	if err != nil {
		t.Fatal(err)
	}
	if got := segments[0].Name; got != "sales/eu" {
		t.Fatalf("database = %q", got)
	}
	data, err := json.Marshal(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ScopePath
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != path {
		t.Fatalf("decoded = %q, want %q", decoded, path)
	}
}

func TestScopePathRejectsInvalidJSONSegments(t *testing.T) {
	for _, input := range []string{
		`[{"kind":"","name":"analytics"}]`,
		`[{"kind":"database","name":""}]`,
	} {
		var path ScopePath
		if err := json.Unmarshal([]byte(input), &path); err == nil {
			t.Fatalf("Unmarshal(%s) unexpectedly succeeded", input)
		}
	}
	data, err := json.Marshal(ScopePath(""))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[]" {
		t.Fatalf("empty scope JSON = %s, want []", data)
	}
}

func TestDirectoryFromCatalogPreservesCheapReferences(t *testing.T) {
	catalog := &Catalog{
		Dialect: "postgres", Database: "app", DefaultNamespace: "public",
		Namespaces: []NamespaceCatalog{{
			Name: "public",
			Groups: []ObjectGroupCatalog{{
				Kind:    "table",
				Objects: []ObjectRef{{Namespace: "public", Kind: "table", Name: "users"}},
			}},
		}},
	}
	directory := DirectoryFromCatalog(catalog)
	if got := directory.DefaultScope.Name("schema"); got != "public" {
		t.Fatalf("default schema = %q", got)
	}
	ref := directory.Roots[0].Children[0].Groups[0].Objects[0]
	if ref.Scope.Name("database") != "app" || ref.Scope.Name("schema") != "public" {
		t.Fatalf("scope = %v", ref.Scope)
	}
}

func TestDirectoryFromCatalogUsesDatabaseLevelForMySQLAndSQLite(t *testing.T) {
	for _, dialect := range []string{"mysql", "sqlite"} {
		t.Run(dialect, func(t *testing.T) {
			catalog := &Catalog{
				Dialect: dialect, Database: "main", DefaultNamespace: "tenant",
				Namespaces: []NamespaceCatalog{{
					Name: "tenant",
					Groups: []ObjectGroupCatalog{{
						Kind:    "table",
						Objects: []ObjectRef{{Namespace: "tenant", Kind: "table", Name: "orders"}},
					}},
				}},
			}
			directory := DirectoryFromCatalog(catalog)
			if len(directory.Roots) != 1 || len(directory.Roots[0].Children) != 0 {
				t.Fatalf("database-level directory = %+v", directory.Roots)
			}
			if got := directory.Roots[0].Path.Name("database"); got != "tenant" {
				t.Fatalf("root database = %q", got)
			}
			if got := directory.Roots[0].Groups[0].Objects[0].Scope; got != directory.Roots[0].Path {
				t.Fatalf("object scope = %q, want %q", got, directory.Roots[0].Path)
			}
			if directory.DefaultScope != directory.Roots[0].Path {
				t.Fatalf("default scope = %q, want %q", directory.DefaultScope, directory.Roots[0].Path)
			}
		})
	}
}

func TestObjectMarshalsRelationalOnly(t *testing.T) {
	o := Object{
		Ref: ObjectRef{Namespace: "public", Kind: "table", Name: "users"},
		Relational: &RelationalDetail{
			Columns:    []Column{{Name: "id", DataType: "int8", Ordinal: 1}},
			PrimaryKey: []string{"id"},
			ForeignKeys: []ForeignKey{{
				Name:              "users_org_fkey",
				Columns:           []string{"org_id"},
				References:        ObjectRef{Namespace: "billing", Kind: "table", Name: "orgs"},
				ReferencedColumns: []string{"id"},
			}},
		},
	}
	data, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"relational"`) {
		t.Errorf("expected relational facet, got %s", s)
	}
	if strings.Contains(s, `"descriptors"`) {
		t.Errorf("descriptors should be omitted when empty: %s", s)
	}
	if !strings.Contains(s, `"references":{"namespace":"billing","kind":"table","name":"orgs"}`) {
		t.Errorf("FK reference must be a qualified ObjectRef: %s", s)
	}
}

func TestObjectMarshalsDescriptorsOnly(t *testing.T) {
	o := Object{
		Ref: ObjectRef{Namespace: "public", Kind: "function", Name: "f"},
		Descriptors: []Descriptor{
			{Kind: "fields", Title: "Signature", Fields: []Field{{Name: "language", Value: "sql"}}},
			{Kind: "source", Title: "Definition", Source: &Source{Language: "sql", Body: "SELECT 1"}},
		},
	}
	data, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round Object
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if round.Relational != nil {
		t.Errorf("relational should be nil, got %+v", round.Relational)
	}
	if len(round.Descriptors) != 2 || round.Descriptors[1].Source.Body != "SELECT 1" {
		t.Errorf("descriptors did not round-trip: %+v", round.Descriptors)
	}
}
