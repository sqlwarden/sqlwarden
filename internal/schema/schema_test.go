package schema

import (
	"encoding/json"
	"testing"
)

func TestSchemaJSONRoundTrip(t *testing.T) {
	def := "0"
	in := &Schema{
		Connection: "42",
		Database:   "app",
		Namespaces: []Namespace{{
			Name: "public",
			Tables: []Table{{
				Name:       "users",
				PrimaryKey: []string{"id"},
				Columns: []Column{
					{Name: "id", DataType: "bigint", Nullable: false, Ordinal: 1},
					{Name: "status", DataType: "text", Nullable: true, Default: &def, Ordinal: 2},
				},
				ForeignKeys: []ForeignKey{{
					Name: "fk_org", Columns: []string{"org_id"},
					ReferencedTable: "orgs", ReferencedColumns: []string{"id"},
				}},
				Indexes:    []Index{{Name: "users_pkey", Columns: []string{"id"}, Unique: true}},
				Attributes: map[string]any{"comment": "people"},
			}},
			Views: []View{{Name: "active_users"}},
		}},
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Schema
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Namespaces[0].Tables[0].Columns[1].Default == nil || *out.Namespaces[0].Tables[0].Columns[1].Default != "0" {
		t.Fatalf("default not preserved")
	}
	if out.Namespaces[0].Tables[0].Attributes["comment"] != "people" {
		t.Fatalf("attributes not preserved")
	}
}
