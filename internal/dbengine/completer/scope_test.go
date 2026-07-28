package completer

import (
	"testing"

	"github.com/sqlwarden/internal/dbengine/schema"
)

func TestScopedColumnsQualifiedAlias(t *testing.T) {
	objects := scopeTestObjects()
	sql := "SELECT * FROM inventory i JOIN store s ON i.id = s.id WHERE s.na"
	columns, handled := ScopedColumns(sql, len(sql), objects, "postgres")
	if !handled {
		t.Fatal("qualified reference was not handled")
	}
	if hasScopedColumn(columns, "inventory_name") || !hasScopedColumn(columns, "store_name") {
		t.Fatalf("qualified columns = %+v", columns)
	}
}

func TestScopedColumnsAliasHidesTableAndUnknownDoesNotLeak(t *testing.T) {
	objects := scopeTestObjects()
	for _, sql := range []string{
		"SELECT * FROM store s WHERE store.",
		"SELECT * FROM store s WHERE missing.",
	} {
		columns, handled := ScopedColumns(sql, len(sql), objects, "postgres")
		if !handled || len(columns) != 0 {
			t.Fatalf("ScopedColumns(%q) = %+v, %v; want handled empty result", sql, columns, handled)
		}
	}
}

func TestScopedColumnsQualifiesAmbiguousJoinColumns(t *testing.T) {
	objects := scopeTestObjects()
	sql := "SELECT * FROM inventory i JOIN store s ON i.id = s.id WHERE "
	columns, handled := ScopedColumns(sql, len(sql), objects, "postgres")
	if !handled {
		t.Fatal("unqualified join was not handled")
	}
	var owners []string
	for _, column := range columns {
		if column.Name == "id" && column.Qualified {
			owners = append(owners, column.Owner)
		}
	}
	if len(owners) != 2 || owners[0] != "i" || owners[1] != "s" {
		t.Fatalf("ambiguous id owners = %v; columns=%+v", owners, columns)
	}
}

func TestScopedColumnsDoesNotUseSiblingSubqueryRelations(t *testing.T) {
	objects := append(scopeTestObjects(), schema.Object{
		Ref: schema.ObjectRef{Namespace: "public", Kind: "table", Name: "audit"},
		Relational: &schema.RelationalDetail{Columns: []schema.Column{
			{Name: "audit_only", DataType: "text"},
		}},
	})
	sql := "SELECT * FROM store s WHERE EXISTS (SELECT 1 FROM audit a WHERE a.audit_only IS NOT NULL) AND s."
	columns, handled := ScopedColumns(sql, len(sql), objects, "postgres")
	if !handled || hasScopedColumn(columns, "audit_only") || !hasScopedColumn(columns, "store_name") {
		t.Fatalf("outer-scope columns = %+v, handled=%v", columns, handled)
	}
}

func TestScopedColumnsResolvesCorrelatedOuterAlias(t *testing.T) {
	objects := append(scopeTestObjects(), schema.Object{
		Ref: schema.ObjectRef{Namespace: "public", Kind: "table", Name: "audit"},
		Relational: &schema.RelationalDetail{Columns: []schema.Column{
			{Name: "audit_only", DataType: "text"},
		}},
	})
	sql := "SELECT * FROM store s WHERE EXISTS (SELECT 1 FROM audit a WHERE s."
	columns, handled := ScopedColumns(sql, len(sql), objects, "postgres")
	if !handled || hasScopedColumn(columns, "audit_only") || !hasScopedColumn(columns, "store_name") {
		t.Fatalf("correlated columns = %+v, handled=%v", columns, handled)
	}
}

func scopeTestObjects() []schema.Object {
	return []schema.Object{
		{
			Ref: schema.ObjectRef{Namespace: "public", Kind: "table", Name: "inventory"},
			Relational: &schema.RelationalDetail{Columns: []schema.Column{
				{Name: "id", DataType: "bigint"}, {Name: "inventory_name", DataType: "text"},
			}},
		},
		{
			Ref: schema.ObjectRef{Namespace: "public", Kind: "table", Name: "store"},
			Relational: &schema.RelationalDetail{Columns: []schema.Column{
				{Name: "id", DataType: "bigint"}, {Name: "store_name", DataType: "text"},
			}},
		},
	}
}

func hasScopedColumn(columns []ScopedColumn, name string) bool {
	for _, column := range columns {
		if column.Name == name {
			return true
		}
	}
	return false
}
