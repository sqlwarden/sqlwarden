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
			ObjectGroups: []ObjectGroup{
				{Kind: "table", Label: "Tables", Objects: []Object{{
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
				}}},
				{Kind: "view", Label: "Views", Objects: []Object{{Name: "active_users"}}},
			},
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
	users := out.Namespaces[0].ObjectGroups[0].Objects[0]
	if users.Columns[1].Default == nil || *users.Columns[1].Default != "0" {
		t.Fatalf("default not preserved")
	}
	if users.Attributes["comment"] != "people" {
		t.Fatalf("attributes not preserved")
	}
}
