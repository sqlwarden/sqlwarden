package build

import (
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

func TestRelationalBuilderQualifiedFK(t *testing.T) {
	b := NewRelational()
	public := metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "public"})
	users := metadata.ObjectRef{Scope: public, Kind: "table", Name: "users"}
	b.AddColumn(users, metadata.Column{Name: "id", DataType: "int8", Ordinal: 1})
	b.AddPrimaryKeyColumn(users, "id")
	b.AddForeignKeyColumn(users, "users_org_fkey", "org_id",
		metadata.ObjectRef{Scope: public.With("schema", "billing"), Kind: "table", Name: "orgs"}, "id")
	b.AddIndex(users, metadata.SecondaryIndex{Name: "users_pkey", Unique: true})

	objs := b.Build()
	if len(objs) != 1 {
		t.Fatalf("want 1 object, got %d", len(objs))
	}
	o := objs[0]
	if o.Ref != users || o.Relational == nil {
		t.Fatalf("ref/facet wrong: %+v", o)
	}
	if len(o.Relational.PrimaryKey) != 1 || o.Relational.PrimaryKey[0] != "id" {
		t.Fatalf("pk wrong: %+v", o.Relational.PrimaryKey)
	}
	fk := o.Relational.ForeignKeys
	if len(fk) != 1 || fk[0].References.Scope.Name("schema") != "billing" || fk[0].References.Name != "orgs" {
		t.Fatalf("FK reference must be qualified, got %+v", fk)
	}
}
