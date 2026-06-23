package sqlite

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/sqlwarden/internal/driver"
	"github.com/sqlwarden/internal/schema"
	"github.com/sqlwarden/pkg/result"
)

func TestSQLiteDriver(t *testing.T) {
	d := &sqliteDriver{}
	ctx := context.Background()

	if err := d.Connect(ctx, driver.ConnectionConfig{DSN: "file:introspect_schema?mode=memory&cache=shared", Driver: "sqlite"}); err != nil {
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

func TestIntrospectCatalogAndObjects(t *testing.T) {
	d := &sqliteDriver{}
	ctx := context.Background()

	if err := d.Connect(ctx, driver.ConnectionConfig{DSN: ":memory:", Driver: "sqlite"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer d.Close()

	if _, err := d.Execute(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("foreign_keys pragma: %v", err)
	}
	if _, err := d.Execute(ctx, `CREATE TABLE introspect_parent (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatalf("create parent: %v", err)
	}
	if _, err := d.Execute(ctx, `
		CREATE TABLE introspect_child (
			id INTEGER PRIMARY KEY,
			parent_id INTEGER NOT NULL REFERENCES introspect_parent(id),
			label TEXT
		)
	`); err != nil {
		t.Fatalf("create child: %v", err)
	}
	if _, err := d.Execute(ctx, `CREATE INDEX idx_child_label ON introspect_child(label)`); err != nil {
		t.Fatalf("create index: %v", err)
	}
	if _, err := d.Execute(ctx, `CREATE VIEW introspect_child_view AS SELECT id, label FROM introspect_child`); err != nil {
		t.Fatalf("create view: %v", err)
	}
	if _, err := d.Execute(ctx, `CREATE TRIGGER introspect_child_ai AFTER INSERT ON introspect_child BEGIN UPDATE introspect_child SET label = COALESCE(label, 'untitled') WHERE id = NEW.id; END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	caps := d.Capabilities()
	if caps.Dialect != "sqlite" || len(caps.Kinds) != 3 {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}

	catalog, err := d.IntrospectCatalog(ctx, schema.CatalogOptions{})
	if err != nil {
		t.Fatalf("IntrospectCatalog: %v", err)
	}
	if !catalogHasRef(catalog, schema.ObjectRef{Namespace: "main", Kind: "table", Name: "introspect_child"}) {
		t.Fatalf("catalog missing child table: %+v", catalog.Namespaces)
	}
	if !catalogHasRef(catalog, schema.ObjectRef{Namespace: "main", Kind: "view", Name: "introspect_child_view"}) {
		t.Fatalf("catalog missing child view: %+v", catalog.Namespaces)
	}
	if !catalogHasRef(catalog, schema.ObjectRef{Namespace: "main", Kind: "trigger", Name: "introspect_child_ai"}) {
		t.Fatalf("catalog missing child trigger: %+v", catalog.Namespaces)
	}

	objects, err := d.IntrospectObjects(ctx, []schema.ObjectRef{{Namespace: "main", Kind: "table", Name: "introspect_child"}})
	if err != nil {
		t.Fatalf("IntrospectObjects: %v", err)
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

	objects, err = d.IntrospectObjects(ctx, []schema.ObjectRef{{Namespace: "main", Kind: "trigger", Name: "introspect_child_ai"}})
	if err != nil {
		t.Fatalf("IntrospectObjects trigger: %v", err)
	}
	if len(objects) != 1 {
		t.Fatalf("expected one trigger, got %d: %+v", len(objects), objects)
	}
	if objects[0].Relational != nil || len(objects[0].Descriptors) == 0 {
		t.Fatalf("expected trigger descriptors, got %+v", objects[0])
	}
}

func TestSQLiteStartQueryCursor(t *testing.T) {
	d := &sqliteDriver{}
	ctx := context.Background()

	if err := d.Connect(ctx, driver.ConnectionConfig{DSN: ":memory:", Driver: "sqlite"}); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer d.Close()

	if _, err := d.Execute(ctx, `CREATE TABLE cursor_users (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatalf("create cursor_users: %v", err)
	}
	if _, err := d.Execute(ctx, `INSERT INTO cursor_users (id, name) VALUES (1, 'Alice'), (2, 'Bob'), (3, 'Cara')`); err != nil {
		t.Fatalf("insert cursor_users: %v", err)
	}

	cursor, err := d.StartQuery(ctx, driver.QueryRequest{SQL: `SELECT id, name FROM cursor_users ORDER BY id`})
	if err != nil {
		t.Fatal(err)
	}
	defer cursor.Close()

	first, state, err := cursor.Fetch(ctx, driver.ScanOptions{MaxRows: 2, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if state.Exhausted || first.RowsReturned != 2 {
		t.Fatalf("first fetch state=%+v result=%+v, want 2 non-exhausted rows", state, first)
	}

	second, state, err := cursor.Fetch(ctx, driver.ScanOptions{MaxRows: 2, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if !state.Exhausted || second.RowsReturned != 1 {
		t.Fatalf("second fetch state=%+v result=%+v, want 1 exhausted row", state, second)
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
			tc.check(t, driver.NormalizeValue(tc.input))
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
