package mysql

import (
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

func TestMySQLSourceDescriptor(t *testing.T) {
	if got := SourceDescriptor("DDL", ""); got != nil {
		t.Fatalf("expected nil descriptor for empty body, got %+v", got)
	}
	got := SourceDescriptor("DDL", "CREATE TABLE t (id int);")
	if got == nil {
		t.Fatal("expected non-nil descriptor")
	}
	if got.Kind != "source" || got.Title != "DDL" {
		t.Fatalf("unexpected descriptor shape: %+v", got)
	}
	if got.Source == nil || got.Source.Language != "sql" || got.Source.Body != "CREATE TABLE t (id int);" {
		t.Fatalf("unexpected descriptor source: %+v", got.Source)
	}
}

func TestMySQLQuoteQualified(t *testing.T) {
	got := mysqlQuoteQualified("my`db", "my`table")
	want := "`my``db`.`my``table`"
	if got != want {
		t.Fatalf("mysqlQuoteQualified() = %q, want %q", got, want)
	}
}

func TestMySQLRequestedRefFallback(t *testing.T) {
	refs := []metadata.ObjectRef{
		{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "app"}), Kind: "table", Name: "users"},
	}
	got := mysqlRequestedRef(refs, "app", "users", "table")
	if got.Name != "users" {
		t.Fatalf("expected exact match, got %+v", got)
	}
	got = mysqlRequestedRef(refs, "app", "missing", "table")
	if got.Name != "missing" || got.Scope.Name("database") != "app" {
		t.Fatalf("expected fallback ref, got %+v", got)
	}
}

func TestMySQLCatalogTablesAndAttachRowCounts(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := t.Context()

	if _, err := d.Execute(ctx, `CREATE TABLE IF NOT EXISTS catalog_tables_test (id INT PRIMARY KEY)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, `DROP TABLE IF EXISTS catalog_tables_test`)
	})

	var database string
	if err := d.DB().QueryRowContext(ctx, `SELECT DATABASE()`).Scan(&database); err != nil {
		t.Fatalf("select database: %v", err)
	}

	type tableRef struct{ schema, name, kind string }
	var tables []tableRef
	if err := CatalogTables(ctx, d.DB(), database, func(schema, name, kind string) {
		tables = append(tables, tableRef{schema, name, kind})
	}); err != nil {
		t.Fatalf("CatalogTables: %v", err)
	}
	found := false
	for _, tbl := range tables {
		if tbl.schema != database {
			t.Fatalf("unexpected schema %q for table %q, want %q", tbl.schema, tbl.name, database)
		}
		if tbl.kind != "table" && tbl.kind != "view" {
			t.Fatalf("unexpected kind %q for table %q", tbl.kind, tbl.name)
		}
		if tbl.name == "catalog_tables_test" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected catalog_tables_test among catalogued tables, got %+v", tables)
	}

	counts := map[string]int64{}
	if err := AttachRowCounts(ctx, d.DB(), database, func(name string, count int64) {
		counts[name] = count
	}); err != nil {
		t.Fatalf("AttachRowCounts: %v", err)
	}
}
