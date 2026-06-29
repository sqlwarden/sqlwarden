package postgres

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/cursor"
	"github.com/sqlwarden/internal/dbengine/schema"
	"github.com/sqlwarden/pkg/result"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

var testDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()

	pgContainer, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("testuser"),
		tcpostgres.WithPassword("testpass"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start postgres container: %v\n", err)
		os.Exit(1)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		_ = pgContainer.Terminate(ctx)
		os.Exit(1)
	}

	testDSN = connStr

	code := m.Run()

	_ = pgContainer.Terminate(ctx)
	os.Exit(code)
}

func newConnectedDriver(t *testing.T) *postgresDriver {
	t.Helper()
	d := &postgresDriver{}
	ctx := context.Background()
	if err := d.Connect(ctx, dbengine.ConnectionConfig{DSN: testDSN, Driver: "postgres"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func currentHeapAlloc() uint64 {
	runtime.GC()
	runtime.GC()

	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

func heapGrowth(before, after uint64) uint64 {
	if after <= before {
		return 0
	}
	return after - before
}

func TestConnect(t *testing.T) {
	t.Run("valid DSN", func(t *testing.T) {
		d := &postgresDriver{}
		ctx := context.Background()
		if err := d.Connect(ctx, dbengine.ConnectionConfig{DSN: testDSN, Driver: "postgres"}); err != nil {
			t.Fatalf("expected connect to succeed, got: %v", err)
		}
		_ = d.Close()
	})

	t.Run("invalid DSN", func(t *testing.T) {
		d := &postgresDriver{}
		ctx := context.Background()
		err := d.Connect(ctx, dbengine.ConnectionConfig{DSN: "postgres://invalid:5432/nonexistent?sslmode=disable", Driver: "postgres"})
		if err == nil {
			_ = d.Close()
			t.Fatal("expected connect to fail with invalid DSN, got nil")
		}
	})
}

func TestPing(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	if err := d.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestQueryCursorDoesNotMaterializeLargeResultSet(t *testing.T) {
	d := newConnectedDriver(t)

	const (
		totalRows        = 200_000
		payloadBytes     = 1024
		pageSize         = 10
		maxHeapGrowthMiB = 64
	)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	before := currentHeapAlloc()
	qc, err := d.StartQuery(ctx, cursor.QueryRequest{SQL: fmt.Sprintf(`
		SELECT n, repeat('x', %d) AS payload
		FROM generate_series(1, %d) AS n
	`, payloadBytes, totalRows)})
	if err != nil {
		t.Fatalf("StartQuery: %v", err)
	}
	defer qc.Close()

	rs, state, err := qc.Fetch(ctx, cursor.ScanOptions{MaxRows: pageSize})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if state.Exhausted {
		t.Fatal("expected cursor to remain open after first page")
	}
	if len(rs.Rows) != pageSize {
		t.Fatalf("expected %d rows, got %d", pageSize, len(rs.Rows))
	}

	after := currentHeapAlloc()
	if growth := heapGrowth(before, after); growth > maxHeapGrowthMiB*1024*1024 {
		t.Fatalf("heap grew by %.2f MiB after fetching %d of %d logical rows; driver may be materializing the full result set", float64(growth)/(1024*1024), pageSize, totalRows)
	}
}

func TestQuery_BasicTypes(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	// Create a table with multiple column types
	_, err := d.Execute(ctx, `
		CREATE TABLE IF NOT EXISTS basic_types_test (
			id        INTEGER,
			label     TEXT,
			active    BOOLEAN,
			price     NUMERIC(10,2),
			created   TIMESTAMPTZ,
			notes     TEXT
		)
	`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS basic_types_test")
	})

	ts := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	_, err = d.Execute(ctx, `
		INSERT INTO basic_types_test (id, label, active, price, created, notes)
		VALUES (1, 'hello', true, 9.99, $1, NULL)
	`, ts)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	rs, err := d.Query(ctx, "SELECT id, label, active, price, created, notes FROM basic_types_test")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	if len(rs.Columns) != 6 {
		t.Errorf("expected 6 columns, got %d", len(rs.Columns))
	}
	if len(rs.Rows) != 1 {
		t.Errorf("expected 1 row, got %d", len(rs.Rows))
	}

	// Verify column names
	expectedCols := []string{"id", "label", "active", "price", "created", "notes"}
	for i, col := range rs.Columns {
		if col.Name != expectedCols[i] {
			t.Errorf("column %d: expected name %q, got %q", i, expectedCols[i], col.Name)
		}
	}

	// Verify NULL value in last column
	if len(rs.Rows) > 0 {
		nullVal := rs.Rows[0][5]
		if nullVal.Type != "null" {
			t.Errorf("expected NULL value in notes column, got type %q", nullVal.Type)
		}
	}
}

func TestQuery_Args(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	_, err := d.Execute(ctx, `
		CREATE TABLE IF NOT EXISTS args_test (
			id    INTEGER,
			name  TEXT
		)
	`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS args_test")
	})

	_, err = d.Execute(ctx, "INSERT INTO args_test (id, name) VALUES (1, 'alpha'), (2, 'beta'), (3, 'gamma')")
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	rs, err := d.Query(ctx, "SELECT id, name FROM args_test WHERE id = $1 AND name = $2", 2, "beta")
	if err != nil {
		t.Fatalf("Query with args: %v", err)
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rs.Rows))
	}
	if rs.Rows[0][1].Text != "beta" {
		t.Errorf("expected name='beta', got %q", rs.Rows[0][1].Text)
	}
}

func TestExecute_DML(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	_, err := d.Execute(ctx, `
		CREATE TABLE IF NOT EXISTS dml_test (
			id   SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS dml_test")
	})

	// Insert multiple rows
	_, err = d.Execute(ctx, "INSERT INTO dml_test (name) VALUES ('row1'), ('row2'), ('row3')")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

	// Verify count via Query
	rs, err := d.Query(ctx, "SELECT COUNT(*) AS cnt FROM dml_test")
	if err != nil {
		t.Fatalf("Query count: %v", err)
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rs.Rows))
	}
	cnt := rs.Rows[0][0].Integer
	if cnt != 3 {
		t.Errorf("expected count=3, got %d", cnt)
	}

	// Test DELETE
	_, err = d.Execute(ctx, "DELETE FROM dml_test WHERE name = 'row1'")
	if err != nil {
		t.Fatalf("DELETE: %v", err)
	}

	rs, err = d.Query(ctx, "SELECT COUNT(*) AS cnt FROM dml_test")
	if err != nil {
		t.Fatalf("Query count after delete: %v", err)
	}
	cnt = rs.Rows[0][0].Integer
	if cnt != 2 {
		t.Errorf("expected count=2 after delete, got %d", cnt)
	}
}

func TestToValue(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	tests := []struct {
		name  string
		input any
		check func(t *testing.T, got result.Value)
	}{
		{
			name:  "nil",
			input: nil,
			check: func(t *testing.T, got result.Value) {
				if got.Type != result.ValueTypeNull {
					t.Fatalf("expected null type, got %v", got.Type)
				}
			},
		},
		{
			name:  "int64",
			input: int64(42),
			check: func(t *testing.T, got result.Value) {
				if got.Type != result.ValueTypeInteger || got.Integer != 42 {
					t.Fatalf("unexpected value: %+v", got)
				}
			},
		},
		{
			name:  "float64",
			input: 3.14,
			check: func(t *testing.T, got result.Value) {
				if got.Type != result.ValueTypeFloat || got.Float != 3.14 {
					t.Fatalf("unexpected value: %+v", got)
				}
			},
		},
		{
			name:  "bool",
			input: true,
			check: func(t *testing.T, got result.Value) {
				if got.Type != result.ValueTypeBool || !got.Bool {
					t.Fatalf("unexpected value: %+v", got)
				}
			},
		},
		{
			name:  "time",
			input: now,
			check: func(t *testing.T, got result.Value) {
				if got.Type != result.ValueTypeTime || got.Time == nil || !got.Time.Equal(now) {
					t.Fatalf("unexpected value: %+v", got)
				}
			},
		},
		{
			name:  "bytes text",
			input: []byte("hello"),
			check: func(t *testing.T, got result.Value) {
				if got.Type != result.ValueTypeText || got.Text != "hello" {
					t.Fatalf("unexpected value: %+v", got)
				}
			},
		},
		{
			name:  "bytes binary",
			input: []byte{0xff, 0xfe},
			check: func(t *testing.T, got result.Value) {
				if got.Type != result.ValueTypeBytes || len(got.Bytes) != 2 {
					t.Fatalf("unexpected value: %+v", got)
				}
			},
		},
		{
			name:  "string",
			input: "sqlwarden",
			check: func(t *testing.T, got result.Value) {
				if got.Type != result.ValueTypeText || got.Text != "sqlwarden" {
					t.Fatalf("unexpected value: %+v", got)
				}
			},
		},
		{
			name:  "fallback",
			input: struct{ N int }{N: 7},
			check: func(t *testing.T, got result.Value) {
				if got.Type != result.ValueTypeText || got.Text == "" {
					t.Fatalf("unexpected value: %+v", got)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, cursor.NormalizeValue(tc.input))
		})
	}
}

func TestDialect(t *testing.T) {
	d := &postgresDriver{}
	if d.Dialect() != dbengine.DialectPostgres {
		t.Errorf("expected dialect %q, got %q", dbengine.DialectPostgres, d.Dialect())
	}
}

func mustExec(t *testing.T, d dbengine.Driver, sql string) {
	t.Helper()
	if _, err := d.Execute(context.Background(), sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func TestPostgresViewDefinitionAndComments(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	mustExec(t, d, `DROP VIEW IF EXISTS defs_v`)
	mustExec(t, d, `DROP TABLE IF EXISTS defs_t`)
	mustExec(t, d, `CREATE TABLE defs_t (id bigint PRIMARY KEY, n text)`)
	mustExec(t, d, `COMMENT ON TABLE defs_t IS 'people table'`)
	mustExec(t, d, `COMMENT ON COLUMN defs_t.n IS 'the name'`)
	mustExec(t, d, `CREATE VIEW defs_v AS SELECT id FROM defs_t`)

	tbl, err := d.InspectObjects(ctx, []schema.ObjectRef{{Namespace: "public", Kind: "table", Name: "defs_t"}})
	if err != nil {
		t.Fatalf("InspectObjects table: %v", err)
	}
	if got := attrString(tbl[0].Attributes, "comment"); got != "people table" {
		t.Fatalf("table comment = %q, want 'people table'", got)
	}
	col := findColumn(tbl[0].Relational.Columns, "n")
	if got := attrString(col.Attributes, "comment"); got != "the name" {
		t.Fatalf("column comment = %q, want 'the name'", got)
	}

	view, err := d.InspectObjects(ctx, []schema.ObjectRef{{Namespace: "public", Kind: "view", Name: "defs_v"}})
	if err != nil {
		t.Fatalf("InspectObjects view: %v", err)
	}
	if def := descriptorByTitle(view[0].Descriptors, "Definition"); def == nil || !strings.Contains(strings.ToUpper(def.Body), "SELECT") {
		t.Fatalf("view definition descriptor missing/blank: %+v", view[0].Descriptors)
	}
}

func TestPostgresTableDDL(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	mustExec(t, d, `DROP TABLE IF EXISTS ddl_child`)
	mustExec(t, d, `DROP TABLE IF EXISTS ddl_parent`)
	mustExec(t, d, `CREATE TABLE ddl_parent (id bigint PRIMARY KEY)`)
	mustExec(t, d, `CREATE TABLE ddl_child (
		id bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
		parent_id bigint NOT NULL REFERENCES ddl_parent(id),
		email text NOT NULL UNIQUE,
		score integer DEFAULT 0 CHECK (score >= 0)
	)`)
	mustExec(t, d, `CREATE INDEX ddl_child_parent_idx ON ddl_child(parent_id)`)

	objs, err := d.InspectObjects(ctx, []schema.ObjectRef{{Namespace: "public", Kind: "table", Name: "ddl_child"}})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	ddl := descriptorByTitle(objs[0].Descriptors, "DDL")
	if ddl == nil {
		t.Fatalf("missing DDL descriptor: %+v", objs[0].Descriptors)
	}
	for _, want := range []string{
		"CREATE TABLE", "ddl_child", "parent_id bigint", "NOT NULL",
		"GENERATED BY DEFAULT AS IDENTITY", "DEFAULT 0",
		"PRIMARY KEY", "FOREIGN KEY", "REFERENCES", "UNIQUE", "CHECK", "CREATE INDEX",
	} {
		if !strings.Contains(ddl.Body, want) {
			t.Fatalf("DDL missing %q:\n%s", want, ddl.Body)
		}
	}
}

// TestPostgresTableDDLRoundTrips proves the generated DDL is both valid and
// faithful: it recreates the tables from our own generated DDL (which fails if
// the SQL is invalid), then regenerates and requires byte-identical output
// (which fails if anything about the structure was lost or reordered). PG's
// auto-generated constraint/index names are deterministic, so an exact match
// means the table round-tripped.
func TestPostgresTableDDLRoundTrips(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	mustExec(t, d, `DROP SCHEMA IF EXISTS ddl_rt CASCADE`)
	mustExec(t, d, `CREATE SCHEMA ddl_rt`)
	t.Cleanup(func() { _, _ = d.Execute(ctx, `DROP SCHEMA IF EXISTS ddl_rt CASCADE`) })

	mustExec(t, d, `CREATE TABLE ddl_rt.parent (id bigint PRIMARY KEY)`)
	mustExec(t, d, `CREATE TABLE ddl_rt.child (
		id bigint GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
		parent_id bigint NOT NULL REFERENCES ddl_rt.parent(id),
		email text NOT NULL UNIQUE,
		score integer DEFAULT 0 CHECK (score >= 0)
	)`)
	mustExec(t, d, `CREATE INDEX child_parent_idx ON ddl_rt.child(parent_id)`)

	parentRef := schema.ObjectRef{Namespace: "ddl_rt", Kind: "table", Name: "parent"}
	childRef := schema.ObjectRef{Namespace: "ddl_rt", Kind: "table", Name: "child"}

	genDDL := func(ref schema.ObjectRef) string {
		objs, err := d.InspectObjects(ctx, []schema.ObjectRef{ref})
		if err != nil {
			t.Fatalf("InspectObjects %s: %v", ref.Name, err)
		}
		src := descriptorByTitle(objs[0].Descriptors, "DDL")
		if src == nil {
			t.Fatalf("missing DDL for %s: %+v", ref.Name, objs[0].Descriptors)
		}
		return src.Body
	}

	parentDDL := genDDL(parentRef)
	childDDL := genDDL(childRef)

	// Recreate from our generated DDL. Statements are separated by blank lines
	// (table, then any CREATE INDEX); each must execute as valid SQL.
	mustExec(t, d, `DROP TABLE ddl_rt.child`)
	mustExec(t, d, `DROP TABLE ddl_rt.parent`)
	for stmt := range strings.SplitSeq(parentDDL, "\n\n") {
		mustExec(t, d, stmt)
	}
	for stmt := range strings.SplitSeq(childDDL, "\n\n") {
		mustExec(t, d, stmt)
	}

	// Regenerating from the recreated tables must be byte-identical.
	if got := genDDL(parentRef); got != parentDDL {
		t.Fatalf("parent DDL not idempotent:\n--- first ---\n%s\n--- second ---\n%s", parentDDL, got)
	}
	if got := genDDL(childRef); got != childDDL {
		t.Fatalf("child DDL not idempotent:\n--- first ---\n%s\n--- second ---\n%s", childDDL, got)
	}
}

func TestPostgresIndexColumns(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	mustExec(t, d, `DROP TABLE IF EXISTS idx_cols`)
	mustExec(t, d, `CREATE TABLE idx_cols (id bigint PRIMARY KEY, a text, b text)`)
	mustExec(t, d, `CREATE INDEX idx_cols_ab ON idx_cols (a, b)`)

	objs, err := d.InspectObjects(ctx, []schema.ObjectRef{{Namespace: "public", Kind: "table", Name: "idx_cols"}})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	var found *schema.Index
	for i := range objs[0].Relational.Indexes {
		if objs[0].Relational.Indexes[i].Name == "idx_cols_ab" {
			found = &objs[0].Relational.Indexes[i]
		}
	}
	if found == nil {
		t.Fatalf("index idx_cols_ab not found: %+v", objs[0].Relational.Indexes)
	}
	if got := strings.Join(found.Columns, ","); got != "a,b" {
		t.Fatalf("index columns = %q, want a,b", got)
	}
}

func attrString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func findColumn(cols []schema.Column, name string) schema.Column {
	for _, c := range cols {
		if c.Name == name {
			return c
		}
	}
	return schema.Column{}
}

func descriptorByTitle(ds []schema.Descriptor, title string) *schema.Source {
	for _, d := range ds {
		if d.Title == title && d.Source != nil {
			return d.Source
		}
	}
	return nil
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestPostgresSchemaSpec(t *testing.T) {
	spec := (&postgresDriver{}).SchemaSpec()
	if spec.Dialect != "postgres" {
		t.Fatalf("dialect: %s", spec.Dialect)
	}
	byKind := map[string]schema.SchemaObjectKind{}
	for _, k := range spec.Kinds {
		byKind[k.Kind] = k
		if k.Listing != "enumerated" {
			t.Errorf("%s listing = %q, want enumerated", k.Kind, k.Listing)
		}
	}
	if !byKind["table"].Relational || !byKind["table"].SupportsDiagram {
		t.Errorf("table must be relational + diagrammable: %+v", byKind["table"])
	}
	if byKind["function"].Relational {
		t.Errorf("function must not be relational")
	}
	for _, want := range []string{"table", "view", "materialized_view", "function", "sequence"} {
		if _, ok := byKind[want]; !ok {
			t.Errorf("missing kind %q", want)
		}
	}
}

func TestPostgresInspectCatalog(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP VIEW IF EXISTS intro_v")
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS intro_users")
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS intro_orgs")
	})
	mustExec(t, d, `CREATE TABLE intro_orgs (id bigint PRIMARY KEY)`)
	mustExec(t, d, `CREATE TABLE intro_users (id bigint PRIMARY KEY, org_id bigint REFERENCES intro_orgs(id))`)
	mustExec(t, d, `CREATE VIEW intro_v AS SELECT id FROM intro_users`)

	cat, err := d.InspectCatalog(ctx, schema.CatalogOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cat.Dialect != "postgres" {
		t.Fatalf("dialect: %s", cat.Dialect)
	}
	var public *schema.NamespaceCatalog
	for i := range cat.Namespaces {
		if cat.Namespaces[i].Name == "public" {
			public = &cat.Namespaces[i]
		}
	}
	if public == nil {
		t.Fatal("no public namespace")
	}
	names := map[string][]string{}
	for _, g := range public.Groups {
		for _, ref := range g.Objects {
			names[g.Kind] = append(names[g.Kind], ref.Name)
		}
	}
	if !contains(names["table"], "intro_users") || !contains(names["table"], "intro_orgs") {
		t.Errorf("missing tables: %+v", names["table"])
	}
	if !contains(names["view"], "intro_v") {
		t.Errorf("missing view: %+v", names["view"])
	}
}

func TestPostgresInspectObjectsRelational(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS intro_users")
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS intro_orgs")
	})
	mustExec(t, d, `CREATE TABLE intro_orgs (id bigint PRIMARY KEY)`)
	mustExec(t, d, `CREATE TABLE intro_users (id bigint PRIMARY KEY, org_id bigint REFERENCES intro_orgs(id))`)

	refs := []schema.ObjectRef{
		{Namespace: "public", Kind: "table", Name: "intro_users"},
		{Namespace: "public", Kind: "table", Name: "intro_orgs"},
	}
	objs, err := d.InspectObjects(ctx, refs)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]schema.Object{}
	for _, o := range objs {
		byName[o.Ref.Name] = o
	}
	users, ok := byName["intro_users"]
	if !ok || users.Relational == nil {
		t.Fatalf("intro_users missing relational facet: %+v", byName)
	}
	if len(users.Relational.PrimaryKey) != 1 || users.Relational.PrimaryKey[0] != "id" {
		t.Errorf("pk: %+v", users.Relational.PrimaryKey)
	}
	if len(users.Relational.ForeignKeys) != 1 {
		t.Fatalf("want 1 fk, got %+v", users.Relational.ForeignKeys)
	}
	ref := users.Relational.ForeignKeys[0].References
	if ref.Namespace != "public" || ref.Kind != "table" || ref.Name != "intro_orgs" {
		t.Errorf("FK reference must be qualified, got %+v", ref)
	}
}

func TestPostgresInspectObjectsHonorsFilter(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS intro_a")
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS intro_b")
	})
	mustExec(t, d, `CREATE TABLE intro_a (id bigint)`)
	mustExec(t, d, `CREATE TABLE intro_b (id bigint)`)

	objs, err := d.InspectObjects(ctx, []schema.ObjectRef{{Namespace: "public", Kind: "table", Name: "intro_a"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 || objs[0].Ref.Name != "intro_a" {
		t.Fatalf("filter not honored, got %+v", objs)
	}
}

func TestPostgresInspectObjectsFunctionDescriptors(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP FUNCTION IF EXISTS intro_add(int, int)")
	})
	mustExec(t, d, `CREATE FUNCTION intro_add(a int, b int) RETURNS int LANGUAGE sql AS 'SELECT a + b'`)

	objs, err := d.InspectObjects(ctx, []schema.ObjectRef{{Namespace: "public", Kind: "function", Name: "intro_add"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 1 {
		t.Fatalf("want 1 object, got %d", len(objs))
	}
	o := objs[0]
	if o.Relational != nil {
		t.Errorf("function must not have a relational facet")
	}
	var hasFields, hasSource bool
	for _, des := range o.Descriptors {
		switch des.Kind {
		case "fields":
			hasFields = true
		case "source":
			hasSource = true
			if des.Source == nil || des.Source.Body == "" {
				t.Errorf("source descriptor empty: %+v", des)
			}
		}
	}
	if !hasFields || !hasSource {
		t.Errorf("function should expose fields + source descriptors, got %+v", o.Descriptors)
	}
}
