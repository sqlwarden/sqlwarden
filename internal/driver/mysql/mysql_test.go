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
	"github.com/sqlwarden/internal/schema"
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
			tc.check(t, toValue(tc.input))
		})
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

func mustExec(t *testing.T, d driver.Driver, sql string) {
	t.Helper()
	if _, err := d.Execute(context.Background(), sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func introTable(t *testing.T, s *schema.Schema, ns, name string) schema.Table {
	t.Helper()
	for _, n := range s.Namespaces {
		if n.Name != ns {
			continue
		}
		for _, tbl := range n.Tables {
			if tbl.Name == name {
				return tbl
			}
		}
	}
	t.Fatalf("table %s.%s not found in %+v", ns, name, s)
	return schema.Table{}
}

func introHasIndex(tbl schema.Table, name string) bool {
	for _, ix := range tbl.Indexes {
		if ix.Name == name {
			return true
		}
	}
	return false
}

func TestMySQLIntrospect(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()

	t.Cleanup(func() {
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS intro_users")
		_, _ = d.Execute(ctx, "DROP TABLE IF EXISTS intro_orgs")
	})
	mustExec(t, d, `CREATE TABLE intro_orgs (id BIGINT PRIMARY KEY)`)
	mustExec(t, d, `CREATE TABLE intro_users (
		id BIGINT PRIMARY KEY,
		org_id BIGINT NOT NULL,
		email VARCHAR(255) NOT NULL,
		CONSTRAINT fk_intro_org FOREIGN KEY (org_id) REFERENCES intro_orgs(id)
	)`)
	mustExec(t, d, `CREATE INDEX intro_users_email_idx ON intro_users(email)`)

	s, err := d.Introspect(ctx, schema.IntrospectOptions{})
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	users := introTable(t, s, s.Namespaces[0].Name, "intro_users")
	if len(users.PrimaryKey) != 1 || users.PrimaryKey[0] != "id" {
		t.Fatalf("expected PK [id], got %v", users.PrimaryKey)
	}
	if len(users.ForeignKeys) != 1 || users.ForeignKeys[0].ReferencedTable != "intro_orgs" {
		t.Fatalf("expected FK to intro_orgs, got %+v", users.ForeignKeys)
	}
	if !introHasIndex(users, "intro_users_email_idx") {
		t.Fatalf("expected intro_users_email_idx index, got %+v", users.Indexes)
	}
}
