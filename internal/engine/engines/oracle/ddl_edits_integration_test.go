//go:build integration

package oracle

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
)

func TestOracleEverydayDDL(t *testing.T) {
	d := newConnectedDriver(t)
	ctx := context.Background()
	ref := &metadata.ObjectRef{Scope: itScope(), Kind: "table", Name: "DDL_EVERYDAY"}
	t.Cleanup(func() { dropQuietly(d, `DROP TABLE DDL_EVERYDAY CASCADE CONSTRAINTS`) })
	apply := func(request ddl.Request) {
		t.Helper()
		if err := d.ApplyDDL(ctx, request); err != nil {
			t.Fatalf("%s: %v", request.Operation, err)
		}
	}
	apply(ddl.Request{Operation: ddl.OperationCreateTable, Scope: itScope(), Name: ref.Name, Columns: []ddl.ColumnDefinition{
		{Name: "ID", DataType: "NUMBER(10)", PrimaryKey: true},
		{Name: "LABEL", DataType: "VARCHAR2(120)", Nullable: true, Default: ddlTestPointer("'initial'")},
	}})
	apply(ddl.Request{Operation: ddl.OperationAddColumn, Ref: ref, Column: &ddl.ColumnDefinition{Name: "AMOUNT", DataType: "NUMBER(10,2)", Default: ddlTestPointer("12.5")}})
	apply(ddl.Request{Operation: ddl.OperationAlterColumn, Ref: ref, Name: "AMOUNT", Changes: &ddl.ColumnChanges{DataType: ddlTestPointer("NUMBER(12,2)")}})
	mustExec(t, d, `INSERT INTO DDL_EVERYDAY (ID) VALUES (1)`)
	var label string
	var amount float64
	if err := d.db.QueryRowContext(ctx, `SELECT LABEL, AMOUNT FROM DDL_EVERYDAY WHERE ID = 1`).Scan(&label, &amount); err != nil {
		t.Fatal(err)
	}
	if label != "initial" || amount != 12.5 {
		t.Fatalf("defaults lost: %q, %v", label, amount)
	}
	var nullable string
	var precision, scale int
	if err := d.db.QueryRowContext(ctx, `SELECT nullable, data_precision, data_scale FROM user_tab_columns WHERE table_name = 'DDL_EVERYDAY' AND column_name = 'AMOUNT'`).Scan(&nullable, &precision, &scale); err != nil {
		t.Fatal(err)
	}
	if nullable != "N" || precision != 12 || scale != 2 {
		t.Fatalf("column definition: %s %d,%d", nullable, precision, scale)
	}
	apply(ddl.Request{Operation: ddl.OperationAlterColumn, Ref: ref, Name: "AMOUNT", Changes: &ddl.ColumnChanges{Nullable: ddlTestPointer(true), Default: ddlTestPointer("NULL")}})
	apply(ddl.Request{Operation: ddl.OperationAlterColumn, Ref: ref, Name: "LABEL", Changes: &ddl.ColumnChanges{Default: ddlTestPointer("'updated'")}})
	mustExec(t, d, `INSERT INTO DDL_EVERYDAY (ID) VALUES (2)`)
	var nullCount int
	if err := d.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM DDL_EVERYDAY WHERE ID = 2 AND AMOUNT IS NULL AND LABEL = 'updated'`).Scan(&nullCount); err != nil || nullCount != 1 {
		t.Fatalf("updated defaults: %d, %v", nullCount, err)
	}
	apply(ddl.Request{Operation: ddl.OperationCreateIndex, Ref: ref, Name: "DDL_EVERYDAY_IX", Unique: true, IndexColumns: []ddl.IndexColumn{{Name: "LABEL"}, {Name: "ID", Descending: true}}})
	var uniqueness string
	if err := d.db.QueryRowContext(ctx, `SELECT uniqueness FROM user_indexes WHERE index_name = 'DDL_EVERYDAY_IX'`).Scan(&uniqueness); err != nil || uniqueness != "UNIQUE" {
		t.Fatalf("unique index: %s, %v", uniqueness, err)
	}
	var firstColumn, direction string
	if err := d.db.QueryRowContext(ctx, `SELECT column_name FROM user_ind_columns WHERE index_name = 'DDL_EVERYDAY_IX' AND column_position = 1`).Scan(&firstColumn); err != nil || firstColumn != "LABEL" {
		t.Fatalf("first index column: %s, %v", firstColumn, err)
	}
	if err := d.db.QueryRowContext(ctx, `SELECT descend FROM user_ind_columns WHERE index_name = 'DDL_EVERYDAY_IX' AND column_position = 2`).Scan(&direction); err != nil || direction != "DESC" {
		t.Fatalf("index direction: %s, %v", direction, err)
	}
}
