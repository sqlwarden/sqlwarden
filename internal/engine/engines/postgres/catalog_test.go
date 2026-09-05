package postgres

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/metadata"
)

func TestSourceDescriptor(t *testing.T) {
	if got := SourceDescriptor("DDL", "sql", ""); got != nil {
		t.Fatalf("expected nil descriptor for empty body, got %+v", got)
	}
	got := SourceDescriptor("DDL", "sql", "CREATE TABLE t (id int);")
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

func TestCatalogTablesComposesStandalone(t *testing.T) {
	ctx := context.Background()
	d := newConnectedDriver(t)
	if _, err := d.DB().ExecContext(ctx, `CREATE TABLE catalog_tables_pg_test (id int)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _, _ = d.DB().ExecContext(ctx, `DROP TABLE IF EXISTS catalog_tables_pg_test`) })

	var got []struct{ schema, name, kind string }
	err := CatalogTables(ctx, d.DB(), func(schema, name, kind string) {
		got = append(got, struct{ schema, name, kind string }{schema, name, kind})
	})
	if err != nil {
		t.Fatalf("CatalogTables: %v", err)
	}
	found := false
	for _, tbl := range got {
		if tbl.name != "catalog_tables_pg_test" {
			continue
		}
		found = true
		if tbl.schema != "public" || tbl.kind != "table" {
			t.Fatalf("unexpected catalog entry for catalog_tables_pg_test: %+v", tbl)
		}
	}
	if !found {
		t.Fatalf("expected catalog_tables_pg_test among catalogued tables, got %+v", got)
	}
}

func TestCatalogMaterializedViewsComposesStandalone(t *testing.T) {
	ctx := context.Background()
	d := newConnectedDriver(t)
	if _, err := d.DB().ExecContext(ctx, `CREATE MATERIALIZED VIEW catalog_mv_pg_test AS SELECT 1 AS id`); err != nil {
		t.Fatalf("create materialized view: %v", err)
	}
	t.Cleanup(func() { _, _ = d.DB().ExecContext(ctx, `DROP MATERIALIZED VIEW IF EXISTS catalog_mv_pg_test`) })

	var got []struct{ schema, name string }
	err := CatalogMaterializedViews(ctx, d.DB(), func(schema, name string) {
		got = append(got, struct{ schema, name string }{schema, name})
	})
	if err != nil {
		t.Fatalf("CatalogMaterializedViews: %v", err)
	}
	found := false
	for _, mv := range got {
		if mv.name == "catalog_mv_pg_test" {
			found = true
			if mv.schema != "public" {
				t.Fatalf("unexpected schema for catalog_mv_pg_test: %+v", mv)
			}
		}
	}
	if !found {
		t.Fatalf("expected catalog_mv_pg_test among catalogued materialized views, got %+v", got)
	}
}

func TestCatalogFunctionsComposesStandalone(t *testing.T) {
	ctx := context.Background()
	d := newConnectedDriver(t)
	if _, err := d.DB().ExecContext(ctx, `CREATE FUNCTION catalog_fn_pg_test() RETURNS int LANGUAGE sql AS 'SELECT 1'`); err != nil {
		t.Fatalf("create function: %v", err)
	}
	t.Cleanup(func() { _, _ = d.DB().ExecContext(ctx, `DROP FUNCTION IF EXISTS catalog_fn_pg_test()`) })

	var got []struct{ schema, name string }
	err := CatalogFunctions(ctx, d.DB(), func(schema, name string) {
		got = append(got, struct{ schema, name string }{schema, name})
	})
	if err != nil {
		t.Fatalf("CatalogFunctions: %v", err)
	}
	found := false
	for _, fn := range got {
		if fn.name == "catalog_fn_pg_test" {
			found = true
			if fn.schema != "public" {
				t.Fatalf("unexpected schema for catalog_fn_pg_test: %+v", fn)
			}
		}
	}
	if !found {
		t.Fatalf("expected catalog_fn_pg_test among catalogued functions, got %+v", got)
	}
}

func TestCatalogSequencesComposesStandalone(t *testing.T) {
	ctx := context.Background()
	d := newConnectedDriver(t)
	if _, err := d.DB().ExecContext(ctx, `CREATE SEQUENCE catalog_seq_pg_test`); err != nil {
		t.Fatalf("create sequence: %v", err)
	}
	t.Cleanup(func() { _, _ = d.DB().ExecContext(ctx, `DROP SEQUENCE IF EXISTS catalog_seq_pg_test`) })

	var got []struct{ schema, name string }
	err := CatalogSequences(ctx, d.DB(), func(schema, name string) {
		got = append(got, struct{ schema, name string }{schema, name})
	})
	if err != nil {
		t.Fatalf("CatalogSequences: %v", err)
	}
	found := false
	for _, seq := range got {
		if seq.name == "catalog_seq_pg_test" {
			found = true
			if seq.schema != "public" {
				t.Fatalf("unexpected schema for catalog_seq_pg_test: %+v", seq)
			}
		}
	}
	if !found {
		t.Fatalf("expected catalog_seq_pg_test among catalogued sequences, got %+v", got)
	}
}

func TestAttachRowCountsComposesStandalone(t *testing.T) {
	ctx := context.Background()
	d := newConnectedDriver(t)
	if _, err := d.DB().ExecContext(ctx, `CREATE TABLE catalog_rowcount_pg_test (id int)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() { _, _ = d.DB().ExecContext(ctx, `DROP TABLE IF EXISTS catalog_rowcount_pg_test`) })
	if _, err := d.DB().ExecContext(ctx, `INSERT INTO catalog_rowcount_pg_test (id) SELECT generate_series(1, 5)`); err != nil {
		t.Fatalf("insert rows: %v", err)
	}
	if _, err := d.DB().ExecContext(ctx, `ANALYZE catalog_rowcount_pg_test`); err != nil {
		t.Fatalf("analyze: %v", err)
	}

	type countKey struct {
		schema, kind, name string
	}
	counts := map[countKey]int64{}
	err := AttachRowCounts(ctx, d.DB(), func(schema, kind, name string, count int64) {
		counts[countKey{schema, kind, name}] = count
	})
	if err != nil {
		t.Fatalf("AttachRowCounts: %v", err)
	}
	key := countKey{"public", "table", "catalog_rowcount_pg_test"}
	count, ok := counts[key]
	if !ok {
		t.Fatalf("expected row count entry for catalog_rowcount_pg_test, got %+v", counts)
	}
	if count != 5 {
		t.Fatalf("row count for catalog_rowcount_pg_test = %d, want 5", count)
	}
}

func TestPostgresRequestedRefFallback(t *testing.T) {
	refs := []metadata.ObjectRef{
		{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "public"}), Kind: "table", Name: "users"},
	}
	got := postgresRequestedRef(refs, "public", "users", "table")
	if got.Name != "users" || got.Kind != "table" {
		t.Fatalf("expected exact match returned, got %+v", got)
	}
	got = postgresRequestedRef(refs, "public", "missing", "table")
	if got.Name != "missing" {
		t.Fatalf("expected fallback ref for unmatched name, got %+v", got)
	}
}
