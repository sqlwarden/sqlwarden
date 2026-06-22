package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/internal/schema"
	"github.com/sqlwarden/pkg/result"
)

func mustExec(t *testing.T, d driver.Driver, sql string) {
	t.Helper()
	if _, err := d.Execute(context.Background(), sql); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func introObject(t *testing.T, s *schema.Schema, ns, kind, name string) schema.Object {
	t.Helper()
	for _, n := range s.Namespaces {
		if n.Name != ns {
			continue
		}
		for _, g := range n.ObjectGroups {
			if g.Kind != kind {
				continue
			}
			for _, o := range g.Objects {
				if o.Name == name {
					return o
				}
			}
		}
	}
	t.Fatalf("%s %s.%s not found in %+v", kind, ns, name, s)
	return schema.Object{}
}

func introHasIndex(o schema.Object, name string) bool {
	for _, ix := range o.Indexes {
		if ix.Name == name {
			return true
		}
	}
	return false
}

func TestSQLiteIntrospect(t *testing.T) {
	d := &sqliteDriver{}
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "introspect.db")
	if err := d.Connect(ctx, driver.ConnectionConfig{DSN: dsn, Driver: "sqlite"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	mustExec(t, d, `CREATE TABLE intro_orgs (id INTEGER PRIMARY KEY)`)
	mustExec(t, d, `CREATE TABLE intro_users (
		id INTEGER PRIMARY KEY,
		org_id INTEGER NOT NULL REFERENCES intro_orgs(id),
		email TEXT NOT NULL
	)`)
	mustExec(t, d, `CREATE INDEX intro_users_email_idx ON intro_users(email)`)

	s, err := d.Introspect(ctx, schema.IntrospectOptions{})
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	users := introObject(t, s, "main", "table", "intro_users")
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

func TestSQLiteIntrospectView(t *testing.T) {
	d := &sqliteDriver{}
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "introspect_view.db")
	if err := d.Connect(ctx, driver.ConnectionConfig{DSN: dsn, Driver: "sqlite"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	mustExec(t, d, `CREATE TABLE intro_accounts (id INTEGER PRIMARY KEY, email TEXT NOT NULL)`)
	mustExec(t, d, `CREATE VIEW intro_active_accounts AS SELECT id, email FROM intro_accounts`)

	s, err := d.Introspect(ctx, schema.IntrospectOptions{})
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}

	var view *schema.Object
	for i := range s.Namespaces {
		for _, g := range s.Namespaces[i].ObjectGroups {
			if g.Kind != "view" {
				continue
			}
			for j := range g.Objects {
				if g.Objects[j].Name == "intro_active_accounts" {
					view = &g.Objects[j]
				}
			}
		}
	}
	if view == nil {
		t.Fatalf("view intro_active_accounts not found in views; schema=%+v", s)
	}
	if len(view.Columns) != 2 {
		t.Fatalf("expected 2 columns on view, got %d (%+v)", len(view.Columns), view.Columns)
	}

	// A view must not also appear among the tables.
	for _, ns := range s.Namespaces {
		for _, g := range ns.ObjectGroups {
			if g.Kind != "table" {
				continue
			}
			for _, o := range g.Objects {
				if o.Name == "intro_active_accounts" {
					t.Fatal("view should not be listed as a table")
				}
			}
		}
	}
}

func TestSQLiteIntrospectTrigger(t *testing.T) {
	d := &sqliteDriver{}
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "introspect_trigger.db")
	if err := d.Connect(ctx, driver.ConnectionConfig{DSN: dsn, Driver: "sqlite"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	mustExec(t, d, `CREATE TABLE intro_log (id INTEGER PRIMARY KEY, n INTEGER)`)
	mustExec(t, d, `CREATE TRIGGER intro_bump AFTER INSERT ON intro_log BEGIN UPDATE intro_log SET n = n + 1; END`)

	s, err := d.Introspect(ctx, schema.IntrospectOptions{})
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	trg := introObject(t, s, "main", "trigger", "intro_bump")
	if trg.Attributes["table"] != "intro_log" {
		t.Fatalf("expected trigger on intro_log, got attributes %+v", trg.Attributes)
	}
}

func TestSQLiteDriver(t *testing.T) {
	d := &sqliteDriver{}
	ctx := context.Background()

	if err := d.Connect(ctx, driver.ConnectionConfig{DSN: ":memory:", Driver: "sqlite"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer d.Close()

	if err := d.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	// Create a test table.
	_, err := d.Execute(ctx, `CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, age INTEGER)`)
	if err != nil {
		t.Fatalf("Create table: %v", err)
	}

	// Insert rows.
	_, err = d.Execute(ctx, `INSERT INTO users (id, name, age) VALUES (1, 'Alice', 30), (2, 'Bob', 25)`)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// Query rows.
	rs, err := d.Query(ctx, `SELECT id, name, age FROM users ORDER BY id`)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(rs.Columns) != 3 {
		t.Errorf("expected 3 columns, got %d", len(rs.Columns))
	}
	if len(rs.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rs.Rows))
	}

	// Test Dialect.
	if d.Dialect() != driver.DialectSQLite {
		t.Errorf("expected dialect sqlite, got %s", d.Dialect())
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
			name:  "fallback string",
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
