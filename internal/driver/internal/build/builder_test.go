package build

import (
	"testing"

	"github.com/sqlwarden/internal/schema"
)

func TestCatalogBuilderOrdersGroupsByDeclaration(t *testing.T) {
	b := NewCatalog()
	b.DeclareKind("table")
	b.DeclareKind("view")
	b.AddRef("public", "view", "v1")
	b.AddRef("public", "table", "t1")
	b.AddRef("public", "table", "t2")

	cat := b.Build("conn", "postgres", "app")
	if cat.Dialect != "postgres" || cat.Database != "app" {
		t.Fatalf("header wrong: %+v", cat)
	}
	if len(cat.Namespaces) != 1 || len(cat.Namespaces[0].Groups) != 2 {
		t.Fatalf("want 1 ns / 2 groups, got %+v", cat.Namespaces)
	}
	g := cat.Namespaces[0].Groups
	if g[0].Kind != "table" || g[1].Kind != "view" {
		t.Fatalf("groups must follow declared order, got %s,%s", g[0].Kind, g[1].Kind)
	}
	if len(g[0].Objects) != 2 || g[0].Objects[0].Name != "t1" {
		t.Fatalf("refs wrong/out of order: %+v", g[0].Objects)
	}
}

func TestRelationalBuilderQualifiedFK(t *testing.T) {
	b := NewRelational()
	users := schema.ObjectRef{Namespace: "public", Kind: "table", Name: "users"}
	b.AddColumn(users, schema.Column{Name: "id", DataType: "int8", Ordinal: 1})
	b.AddPrimaryKeyColumn(users, "id")
	b.AddForeignKeyColumn(users, "users_org_fkey", "org_id",
		schema.ObjectRef{Namespace: "billing", Kind: "table", Name: "orgs"}, "id")
	b.AddIndex(users, schema.Index{Name: "users_pkey", Unique: true})

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
	if len(fk) != 1 || fk[0].References.Namespace != "billing" || fk[0].References.Name != "orgs" {
		t.Fatalf("FK reference must be qualified, got %+v", fk)
	}
}
