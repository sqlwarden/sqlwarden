package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

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
