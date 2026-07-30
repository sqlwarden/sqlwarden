package build

import (
	"testing"

	"github.com/sqlwarden/internal/dbengine/schema"
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

func TestCatalogBuilderBuildsArbitraryScopeDepth(t *testing.T) {
	b := NewCatalog()
	root := schema.NewScopePath(schema.ScopeSegment{Kind: "cluster", Name: "primary"})
	database := root.Child(schema.ScopeSegment{Kind: "database", Name: "analytics"})
	nested := database.Child(schema.ScopeSegment{Kind: "schema", Name: "reporting"})
	b.AddRef(root, "cluster", "primary")
	b.AddRef(database, "database", "analytics")
	b.AddRef(nested, "table", "orders")

	directory := b.BuildDirectory("conn", "future-engine", nested)
	if directory.DefaultScope != nested || len(directory.Roots) != 1 {
		t.Fatalf("directory header/roots = %+v", directory)
	}
	databaseNode := directory.Roots[0].Children
	if len(databaseNode) != 1 || len(databaseNode[0].Children) != 1 {
		t.Fatalf("nested hierarchy = %+v", directory.Roots)
	}
	tableNode := databaseNode[0].Children[0]
	if tableNode.Path != nested || tableNode.Groups[0].Objects[0].Scope != nested {
		t.Fatalf("nested object scope = %+v", tableNode)
	}
}

func TestRelationalBuilderQualifiedFK(t *testing.T) {
	b := NewRelational()
	users := schema.ObjectRef{Namespace: "public", Kind: "table", Name: "users"}
	b.AddColumn(users, schema.Column{Name: "id", DataType: "int8", Ordinal: 1})
	b.AddPrimaryKeyColumn(users, "id")
	b.AddForeignKeyColumn(users, "users_org_fkey", "org_id",
		schema.ObjectRef{Namespace: "billing", Kind: "table", Name: "orgs"}, "id")
	b.AddIndex(users, schema.SecondaryIndex{Name: "users_pkey", Unique: true})

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
