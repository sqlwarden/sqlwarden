package sqlite

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
)

func TestSQLiteDDLsRoundTrip(t *testing.T) {
	driver := &sqliteDriver{}
	if err := driver.Connect(context.Background(), engine.ConnectionConfig{DSN: filepath.Join(t.TempDir(), "ddl.db")}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = driver.Close() })
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "main"})
	table := metadata.ObjectRef{Scope: scope, Kind: "table", Name: "events"}
	requests := []ddl.Request{
		{Operation: ddl.OperationCreateTable, Scope: scope, Name: table.Name, Columns: []ddl.ColumnDefinition{{Name: "id", DataType: "integer", PrimaryKey: true}, {Name: "note", DataType: "text", Nullable: true}}},
		{Operation: ddl.OperationRenameColumn, Ref: &table, Name: "note", NewName: "message"},
		{Operation: ddl.OperationDropColumn, Ref: &table, Name: "message"},
		{Operation: ddl.OperationDropObject, Ref: &table},
	}
	for _, request := range requests {
		if err := driver.ApplyDDL(context.Background(), request); err != nil {
			t.Fatalf("%s: %v", request.Operation, err)
		}
	}
	q, err := driver.Querier(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	tables, err := ListTables(context.Background(), q, []metadata.ScopePath{scope})
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(tables[scope], func(c metadata.Child) bool { return c.Name == table.Name }) {
		t.Fatalf("tables after drop = %+v", tables[scope])
	}
}
