package mysql

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/sqlwarden/internal/driver"
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
	if err := d.Connect(ctx, driver.ConnectionConfig{DSN: testDSN, Driver: "mysql"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestConnect(t *testing.T) {
	t.Run("valid DSN", func(t *testing.T) {
		d := &mysqlDriver{}
		ctx := context.Background()
		if err := d.Connect(ctx, driver.ConnectionConfig{DSN: testDSN, Driver: "mysql"}); err != nil {
			t.Fatalf("expected connect to succeed, got: %v", err)
		}
		t.Cleanup(func() { _ = d.Close() })
	})

	t.Run("invalid DSN", func(t *testing.T) {
		d := &mysqlDriver{}
		ctx := context.Background()
		err := d.Connect(ctx, driver.ConnectionConfig{DSN: "testuser:testpass@tcp(127.0.0.1:19999)/nonexistent", Driver: "mysql"})
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

func TestTables(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	_, err := d.Execute(ctx, "CREATE TABLE IF NOT EXISTS tables_test_a (id INT)")
	if err != nil {
		t.Fatalf("create table a: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS tables_test_a")
	})

	_, err = d.Execute(ctx, "CREATE TABLE IF NOT EXISTS tables_test_b (id INT)")
	if err != nil {
		t.Fatalf("create table b: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS tables_test_b")
	})

	tables, err := d.Tables(ctx, "", "")
	if err != nil {
		t.Fatalf("Tables: %v", err)
	}

	findTable := func(list []driver.TableMeta, name string) bool {
		for _, tb := range list {
			if tb.Name == name {
				return true
			}
		}
		return false
	}

	if !findTable(tables, "tables_test_a") {
		t.Errorf("tables_test_a not found in Tables() result")
	}
	if !findTable(tables, "tables_test_b") {
		t.Errorf("tables_test_b not found in Tables() result")
	}

	// Filter by schema = "testdb" — should still find our tables
	filteredTables, err := d.Tables(ctx, "", "testdb")
	if err != nil {
		t.Fatalf("Tables with schema filter: %v", err)
	}
	if !findTable(filteredTables, "tables_test_a") {
		t.Errorf("tables_test_a not found when filtering by testdb schema")
	}
	if !findTable(filteredTables, "tables_test_b") {
		t.Errorf("tables_test_b not found when filtering by testdb schema")
	}

	// Filter by non-existent schema — should return empty
	noneTables, err := d.Tables(ctx, "", "nonexistent_schema_xyz")
	if err != nil {
		t.Fatalf("Tables with nonexistent schema: %v", err)
	}
	if len(noneTables) != 0 {
		t.Errorf("expected 0 tables for nonexistent schema, got %d", len(noneTables))
	}

	// Verify each returned table has a non-empty schema
	for _, tb := range tables {
		if tb.Schema == "" {
			t.Errorf("table %q has empty schema", tb.Name)
		}
	}
}

func TestColumns(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	_, err := d.Execute(ctx, `
		CREATE TABLE IF NOT EXISTS columns_test (
			id     INT NOT NULL,
			name   VARCHAR(255),
			price  DECIMAL(10,2),
			active TINYINT(1) NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS columns_test")
	})

	cols, err := d.Columns(ctx, "", "testdb", "columns_test")
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}
	if len(cols) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(cols))
	}

	colMap := make(map[string]driver.ColumnMeta)
	for _, c := range cols {
		colMap[c.Name] = c
	}

	// id — INT NOT NULL
	idCol, ok := colMap["id"]
	if !ok {
		t.Fatal("column 'id' not found")
	}
	if idCol.Nullable {
		t.Errorf("column 'id' should not be nullable")
	}

	// name — VARCHAR(255) (nullable)
	nameCol, ok := colMap["name"]
	if !ok {
		t.Fatal("column 'name' not found")
	}
	if !nameCol.Nullable {
		t.Errorf("column 'name' should be nullable")
	}

	// active — TINYINT(1) NOT NULL
	activeCol, ok := colMap["active"]
	if !ok {
		t.Fatal("column 'active' not found")
	}
	if activeCol.Nullable {
		t.Errorf("column 'active' should not be nullable")
	}

	// Verify types are non-empty
	for _, c := range cols {
		if c.Type == "" {
			t.Errorf("column %q has empty type", c.Name)
		}
	}
}

func TestDialect(t *testing.T) {
	d := &mysqlDriver{}
	if d.Dialect() != driver.DialectMySQL {
		t.Errorf("expected dialect %q, got %q", driver.DialectMySQL, d.Dialect())
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
