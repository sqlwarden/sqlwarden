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

func TestObjectMarshalsRelationalOnly(t *testing.T) {
	public := NewScopePath(ScopeSegment{Kind: "database", Name: "app"}, ScopeSegment{Kind: "schema", Name: "public"})
	o := Object{
		Ref: ObjectRef{Scope: public, Kind: "table", Name: "users"},
		Relational: &RelationalDetail{
			Columns:    []Column{{Name: "id", DataType: "int8", Ordinal: 1}},
			PrimaryKey: []string{"id"},
			ForeignKeys: []ForeignKey{{
				Name:              "users_org_fkey",
				Columns:           []string{"org_id"},
				References:        ObjectRef{Scope: public.With("schema", "billing"), Kind: "table", Name: "orgs"},
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
	if !strings.Contains(s, `"scope":[{"kind":"database","name":"app"},{"kind":"schema","name":"billing"}]`) {
		t.Errorf("FK reference must be a qualified ObjectRef: %s", s)
	}
}

func TestObjectMarshalsDescriptorsOnly(t *testing.T) {
	o := Object{
		Ref: ObjectRef{Scope: NewScopePath(ScopeSegment{Kind: "schema", Name: "public"}), Kind: "function", Name: "f"},
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
