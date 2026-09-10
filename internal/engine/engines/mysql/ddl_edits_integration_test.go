package mysql

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
)

// TestMySQLApplyDDLAlterColumnPreservesUnspecifiedAttributes proves that
// ALTER COLUMN requests which only patch a subset of ColumnChanges fields
// (here: data type only) preserve the column's existing NOT NULL and DEFAULT
// against a live database, including a string-literal DEFAULT: MySQL reports
// column_default for string-typed columns unquoted (e.g. DEFAULT 'unnamed'
// reads back as the bare text unnamed), so currentMySQLColumn must re-quote
// it before mysqlAlterColumnSQL re-emits it verbatim, or the MODIFY COLUMN
// statement is a live SQL syntax error.
func TestMySQLApplyDDLAlterColumnPreservesUnspecifiedAttributes(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		dropQuietlyMySQL(t, d, "DROP TABLE IF EXISTS ddl_edits_widgets")
	})

	mustExecMySQL(t, d, `CREATE TABLE ddl_edits_widgets (id INT PRIMARY KEY, label VARCHAR(50) NOT NULL DEFAULT 'unnamed')`)
	ref := metadata.ObjectRef{Scope: mysqlTestScope(), Kind: "table", Name: "ddl_edits_widgets"}

	newType := "varchar(100)"
	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationAlterColumn, Ref: &ref, Name: "label",
		Changes: &ddl.ColumnChanges{DataType: &newType},
	}); err != nil {
		t.Fatalf("alter column: %v", err)
	}

	objs, err := d.InspectObjects(ctx, []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objs) != 1 || objs[0].Relational == nil {
		t.Fatal("expected one relational object")
	}
	var col *metadata.Column
	for i := range objs[0].Relational.Columns {
		if objs[0].Relational.Columns[i].Name == "label" {
			col = &objs[0].Relational.Columns[i]
		}
	}
	if col == nil {
		t.Fatal("expected label column")
	}
	if col.DataType != "varchar(100)" {
		t.Errorf("expected altered type varchar(100), got %q", col.DataType)
	}
	if col.Nullable {
		t.Error("expected NOT NULL to be preserved from the original definition")
	}
	// InspectObjects reads column_default via RelationalObjects, which
	// (like currentMySQLColumn's pre-fix behavior) reports string defaults
	// unquoted, so the preserved value shows up as the bare literal.
	if col.Default == nil || *col.Default != "unnamed" {
		t.Errorf("expected default unnamed to be preserved, got %v", col.Default)
	}
}

// TestMySQLApplyDDLAlterColumnPreservesExpressionDefault proves that an
// expression DEFAULT (CURRENT_TIMESTAMP, reported via
// information_schema.columns.extra containing DEFAULT_GENERATED) survives a
// type-only ALTER COLUMN unrequoted, since it is not a string literal.
func TestMySQLApplyDDLAlterColumnPreservesExpressionDefault(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		dropQuietlyMySQL(t, d, "DROP TABLE IF EXISTS ddl_edits_events")
	})

	mustExecMySQL(t, d, `CREATE TABLE ddl_edits_events (id INT PRIMARY KEY, seen DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	ref := metadata.ObjectRef{Scope: mysqlTestScope(), Kind: "table", Name: "ddl_edits_events"}

	newType := "timestamp"
	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationAlterColumn, Ref: &ref, Name: "seen",
		Changes: &ddl.ColumnChanges{DataType: &newType},
	}); err != nil {
		t.Fatalf("alter column: %v", err)
	}

	objs, err := d.InspectObjects(ctx, []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objs) != 1 || objs[0].Relational == nil {
		t.Fatal("expected one relational object")
	}
	var col *metadata.Column
	for i := range objs[0].Relational.Columns {
		if objs[0].Relational.Columns[i].Name == "seen" {
			col = &objs[0].Relational.Columns[i]
		}
	}
	if col == nil {
		t.Fatal("expected seen column")
	}
	if col.DataType != "timestamp" {
		t.Errorf("expected altered type timestamp, got %q", col.DataType)
	}
	if col.Default == nil || *col.Default != "CURRENT_TIMESTAMP" {
		t.Errorf("expected default CURRENT_TIMESTAMP to be preserved, got %v", col.Default)
	}
}

// TestMySQLApplyDDLCreateIndex proves ApplyDDL's create_index operation adds
// a unique index that InspectObjects then reports against a live database.
func TestMySQLApplyDDLCreateIndex(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		dropQuietlyMySQL(t, d, "DROP TABLE IF EXISTS ddl_edits_index_widgets")
	})

	mustExecMySQL(t, d, `CREATE TABLE ddl_edits_index_widgets (id INT PRIMARY KEY, label VARCHAR(50))`)
	ref := metadata.ObjectRef{Scope: mysqlTestScope(), Kind: "table", Name: "ddl_edits_index_widgets"}

	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationCreateIndex, Ref: &ref, Name: "ddl_edits_index_widgets_label_idx",
		IndexColumns: []ddl.IndexColumn{{Name: "label"}}, Unique: true,
	}); err != nil {
		t.Fatalf("create index: %v", err)
	}

	objs, err := d.InspectObjects(ctx, []metadata.ObjectRef{ref})
	if err != nil {
		t.Fatalf("InspectObjects: %v", err)
	}
	if len(objs) != 1 || objs[0].Relational == nil {
		t.Fatal("expected one relational object")
	}
	var hasIndex bool
	for _, idx := range objs[0].Relational.Indexes {
		if idx.Name == "ddl_edits_index_widgets_label_idx" && idx.Unique {
			hasIndex = true
		}
	}
	if !hasIndex {
		t.Error("expected ddl_edits_index_widgets_label_idx to be present and unique")
	}
}

// mysqlTestScope returns the scope path for the "testdb" database seeded by
// TestMain's testcontainers setup, matching the convention already used
// across this package's other live-DB tests.
func mysqlTestScope() metadata.ScopePath {
	return metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "testdb"})
}
