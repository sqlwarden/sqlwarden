package mysql

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/sqlwarden/internal/dbengine"
	"github.com/sqlwarden/internal/dbengine/cursor"
	"github.com/sqlwarden/internal/dbengine/schema"
	"github.com/sqlwarden/pkg/result"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testDSN string

func TestMain(m *testing.M) {
	ctx := context.Background()

	mysqlContainer, err := tcmysql.Run(ctx,
		"mysql:8.0.36",
		tcmysql.WithConfigFile(filepath.Join("testdata", "my.cnf")),
		tcmysql.WithDatabase("testdb"),
		tcmysql.WithUsername("testuser"),
		tcmysql.WithPassword("testpass"),
		testcontainers.WithTmpfs(map[string]string{
			"/var/lib/mysql": "rw",
		}),
		testcontainers.WithWaitStrategy(
			wait.ForSQL("3306/tcp", "mysql", func(host string, port nat.Port) string {
				return fmt.Sprintf("testuser:testpass@tcp(%s:%s)/testdb", host, port.Port())
			}).WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start mysql container: %v\n", err)
		os.Exit(1)
	}

	connStr, err := mysqlContainer.ConnectionString(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get connection string: %v\n", err)
		_ = mysqlContainer.Terminate(ctx)
		os.Exit(1)
	}

	testDSN = connStr

	code := m.Run()

	_ = mysqlContainer.Terminate(ctx)
	os.Exit(code)
}

func newConnectedDriver(t *testing.T) *mysqlDriver {
	t.Helper()
	d := &mysqlDriver{}
	ctx := context.Background()
	if err := d.Connect(ctx, dbengine.ConnectionConfig{DSN: testDSN, Driver: "mysql"}); err != nil {
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
		d := &mysqlDriver{}
		ctx := context.Background()
		if err := d.Connect(ctx, dbengine.ConnectionConfig{DSN: testDSN, Driver: "mysql"}); err != nil {
			t.Fatalf("expected connect to succeed, got: %v", err)
		}
		t.Cleanup(func() { _ = d.Close() })
	})

	t.Run("invalid DSN", func(t *testing.T) {
		d := &mysqlDriver{}
		ctx := context.Background()
		err := d.Connect(ctx, dbengine.ConnectionConfig{DSN: "testuser:testpass@tcp(127.0.0.1:19999)/nonexistent", Driver: "mysql"})
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
		WITH RECURSIVE seq(n) AS (
			SELECT 1
			UNION ALL
			SELECT n + 1 FROM seq WHERE n < %d
		)
		SELECT /*+ SET_VAR(cte_max_recursion_depth=%d) */ n, REPEAT('x', %d) AS payload
		FROM seq
	`, totalRows, totalRows, payloadBytes)})
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

	_, err := d.Execute(ctx, `
		CREATE TABLE IF NOT EXISTS basic_types_test (
			id        INT,
			label     VARCHAR(255),
			active    TINYINT(1),
			price     DECIMAL(10,2),
			created   DATETIME,
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
		VALUES (1, 'hello', 1, 9.99, ?, NULL)
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
			id    INT,
			name  VARCHAR(255)
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

	rs, err := d.Query(ctx, "SELECT id, name FROM args_test WHERE id = ? AND name = ?", 2, "beta")
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
			id   INT AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(255) NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("CREATE TABLE: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS dml_test")
	})

	_, err = d.Execute(ctx, "INSERT INTO dml_test (name) VALUES ('row1'), ('row2'), ('row3')")
	if err != nil {
		t.Fatalf("INSERT: %v", err)
	}

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

func TestInspectCatalogAndObjects(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	_, _ = d.Execute(ctx, "DROP VIEW IF EXISTS introspect_child_view")
	_, _ = d.Execute(ctx, "DROP TRIGGER IF EXISTS introspect_child_bi")
	_, _ = d.Execute(ctx, "DROP FUNCTION IF EXISTS introspect_double")
	_, _ = d.Execute(ctx, "DROP PROCEDURE IF EXISTS introspect_noop")
	_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS introspect_child")
	_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS introspect_parent")
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP VIEW IF EXISTS introspect_child_view")
		_, _ = d.Execute(ctx, "DROP TRIGGER IF EXISTS introspect_child_bi")
		_, _ = d.Execute(ctx, "DROP FUNCTION IF EXISTS introspect_double")
		_, _ = d.Execute(ctx, "DROP PROCEDURE IF EXISTS introspect_noop")
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS introspect_child")
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS introspect_parent")
	})

	if _, err := d.Execute(ctx, `
		CREATE TABLE introspect_parent (
			id INT PRIMARY KEY,
			name VARCHAR(64) NOT NULL
		)
	`); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if _, err := d.Execute(ctx, `
		CREATE TABLE introspect_child (
			id INT PRIMARY KEY,
			parent_id INT NOT NULL,
			label VARCHAR(64),
			INDEX idx_child_label (label),
			CONSTRAINT fk_child_parent FOREIGN KEY (parent_id) REFERENCES introspect_parent(id)
		)
	`); err != nil {
		t.Fatalf("create child: %v", err)
	}
	if _, err := d.Execute(ctx, `CREATE VIEW introspect_child_view AS SELECT id, label FROM introspect_child`); err != nil {
		t.Fatalf("create view: %v", err)
	}
	if _, err := d.Execute(ctx, `CREATE FUNCTION introspect_double(n INT) RETURNS INT DETERMINISTIC RETURN n * 2`); err != nil {
		t.Fatalf("create function: %v", err)
	}
	if _, err := d.Execute(ctx, `CREATE PROCEDURE introspect_noop() BEGIN SELECT 1; END`); err != nil {
		t.Fatalf("create procedure: %v", err)
	}
	if _, err := d.Execute(ctx, `CREATE TRIGGER introspect_child_bi BEFORE INSERT ON introspect_child FOR EACH ROW SET NEW.label = COALESCE(NEW.label, 'untitled')`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	spec := d.SchemaSpec()
	if spec.Dialect != "mysql" || len(spec.Kinds) != 5 {
		t.Fatalf("unexpected schema spec: %+v", spec)
	}

	catalog, err := d.InspectCatalog(ctx, schema.CatalogOptions{})
	if err != nil {
		t.Fatalf("InspectCatalog: %v", err)
	}
	if catalog.Dialect != "mysql" || catalog.Database != "testdb" {
		t.Fatalf("unexpected catalog header: %+v", catalog)
	}
	if !catalogHasRef(catalog, schema.ObjectRef{Namespace: "testdb", Kind: "table", Name: "introspect_child"}) {
		t.Fatalf("catalog missing child table: %+v", catalog.Namespaces)
	}
	if !catalogHasRef(catalog, schema.ObjectRef{Namespace: "testdb", Kind: "view", Name: "introspect_child_view"}) {
		t.Fatalf("catalog missing child view: %+v", catalog.Namespaces)
	}
	for _, ref := range []schema.ObjectRef{
		{Namespace: "testdb", Kind: "function", Name: "introspect_double"},
		{Namespace: "testdb", Kind: "procedure", Name: "introspect_noop"},
		{Namespace: "testdb", Kind: "trigger", Name: "introspect_child_bi"},
	} {
		if !catalogHasRef(catalog, ref) {
			t.Fatalf("catalog missing %s %s: %+v", ref.Kind, ref.Name, catalog.Namespaces)
		}
	}

	objects, err := d.InspectObjects(ctx, []schema.ObjectRef{{Namespace: "testdb", Kind: "table", Name: "introspect_child"}})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objects) != 1 {
		t.Fatalf("expected one object, got %d: %+v", len(objects), objects)
	}
	child := objects[0]
	if child.Relational == nil {
		t.Fatalf("expected relational detail: %+v", child)
	}
	if !slices.Contains(child.Relational.PrimaryKey, "id") {
		t.Fatalf("expected primary key id, got %+v", child.Relational.PrimaryKey)
	}
	if len(child.Relational.ForeignKeys) != 1 || child.Relational.ForeignKeys[0].References.Name != "introspect_parent" {
		t.Fatalf("expected parent foreign key, got %+v", child.Relational.ForeignKeys)
	}
	if !hasIndex(child.Relational.Indexes, "idx_child_label", "label") {
		t.Fatalf("expected idx_child_label index, got %+v", child.Relational.Indexes)
	}

	objects, err = d.InspectObjects(ctx, []schema.ObjectRef{
		{Namespace: "testdb", Kind: "function", Name: "introspect_double"},
		{Namespace: "testdb", Kind: "procedure", Name: "introspect_noop"},
		{Namespace: "testdb", Kind: "trigger", Name: "introspect_child_bi"},
	})
	if err != nil {
		t.Fatalf("InspectObjects descriptors: %v", err)
	}
	if len(objects) != 3 {
		t.Fatalf("expected three descriptor objects, got %d: %+v", len(objects), objects)
	}
	for _, object := range objects {
		if object.Relational != nil {
			t.Fatalf("expected non-relational %s, got %+v", object.Ref.Kind, object.Relational)
		}
		if len(object.Descriptors) == 0 {
			t.Fatalf("expected descriptors for %s, got %+v", object.Ref.Kind, object)
		}
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

func catalogHasRef(catalog *schema.Catalog, ref schema.ObjectRef) bool {
	for _, ns := range catalog.Namespaces {
		for _, group := range ns.Groups {
			for _, got := range group.Objects {
				if got == ref {
					return true
				}
			}
		}
	}
	return false
}

func hasIndex(indexes []schema.Index, name, column string) bool {
	for _, ix := range indexes {
		if ix.Name == name && slices.Contains(ix.Columns, column) {
			return true
		}
	}
	return false
}

func TestDialect(t *testing.T) {
	d := &mysqlDriver{}
	if d.Dialect() != dbengine.DialectMySQL {
		t.Errorf("expected dialect %q, got %q", dbengine.DialectMySQL, d.Dialect())
	}
}

func TestEnsureParams(t *testing.T) {
	t.Run("no params appended", func(t *testing.T) {
		dsn := "user:pass@tcp(localhost:3306)/mydb"
		result := ensureParams(dsn)
		if result == dsn {
			t.Error("expected params to be added, DSN unchanged")
		}
		if !containsParam(result, "parseTime=true") {
			t.Errorf("expected parseTime=true in DSN %q", result)
		}
	})

	t.Run("parseTime already present", func(t *testing.T) {
		dsn := "user:pass@tcp(localhost:3306)/mydb?parseTime=true"
		result := ensureParams(dsn)
		// parseTime should not be duplicated
		count := strings.Count(result, "parseTime=true")
		if count != 1 {
			t.Errorf("expected parseTime=true to appear exactly once, got %d times in %q", count, result)
		}
		// result should equal original DSN since nothing new is added
		if result != dsn {
			t.Errorf("expected DSN to be unchanged when parseTime already present, got %q", result)
		}
	})

	t.Run("parseTime already present returns unchanged", func(t *testing.T) {
		dsn := "user:pass@tcp(localhost:3306)/mydb?parseTime=true&charset=utf8mb4"
		result := ensureParams(dsn)
		if result != dsn {
			t.Errorf("expected DSN to be unchanged, got %q", result)
		}
	})
}

func containsParam(dsn, param string) bool {
	_, query, ok := strings.Cut(dsn, "?")
	if !ok {
		return false
	}
	parts := strings.Split(query, "&")
	return slices.Contains(parts, param)
}
