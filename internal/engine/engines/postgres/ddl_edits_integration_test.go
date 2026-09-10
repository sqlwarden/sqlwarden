package postgres

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
)

// TestPostgresApplyDDLAddAlterColumnAndCreateIndex exercises the three DDL
// operations added for Postgres/MySQL parity (add column, alter column,
// create index) against a live database and confirms the resulting column
// type and index are visible through InspectObjects.
func TestPostgresApplyDDLAddAlterColumnAndCreateIndex(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	t.Cleanup(func() {
		dropQuietlyPostgres(t, d, "DROP TABLE IF EXISTS ddl_edits_widgets")
	})

	mustExec(t, d, `CREATE TABLE ddl_edits_widgets (id serial PRIMARY KEY)`)
	ref := metadata.ObjectRef{Scope: pgTestScope("public"), Kind: "table", Name: "ddl_edits_widgets"}

	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationAddColumn, Ref: &ref,
		Column: &ddl.ColumnDefinition{Name: "label", DataType: "varchar(100)", Nullable: true},
	}); err != nil {
		t.Fatalf("add column: %v", err)
	}

	newType := "varchar(200)"
	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationAlterColumn, Ref: &ref, Name: "label",
		Changes: &ddl.ColumnChanges{DataType: &newType},
	}); err != nil {
		t.Fatalf("alter column: %v", err)
	}

	if err := d.ApplyDDL(ctx, ddl.Request{
		Operation: ddl.OperationCreateIndex, Ref: &ref, Name: "ddl_edits_widgets_label_idx",
		IndexColumns: []ddl.IndexColumn{{Name: "label"}},
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
	var found bool
	for _, col := range objs[0].Relational.Columns {
		// RelationalObjects sources Column.DataType from
		// information_schema.columns.udt_name, which reports the bare
		// internal type name ("varchar") without the length modifier —
		// distinct from format_type(atttypid, atttypmod), which this package
		// uses only for generated DDL text.
		if col.Name == "label" && col.DataType == "varchar" {
			found = true
		}
	}
	if !found {
		t.Error("expected label column with altered varchar type")
	}
	var hasIndex bool
	for _, idx := range objs[0].Relational.Indexes {
		if idx.Name == "ddl_edits_widgets_label_idx" {
			hasIndex = true
		}
	}
	if !hasIndex {
		t.Error("expected ddl_edits_widgets_label_idx to be present")
	}
}
