package postgres

import (
	"testing"

	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
)

func TestPostgresDDLSQL(t *testing.T) {
	scope := metadata.NewScopePath(
		metadata.ScopeSegment{Kind: "database", Name: "app"},
		metadata.ScopeSegment{Kind: "schema", Name: `tenant"one`},
	)
	table := metadata.ObjectRef{Scope: scope, Kind: "table", Name: `order"items`}
	tests := []struct {
		name string
		req  ddl.Request
		want string
	}{
		{name: "create table", req: ddl.Request{Operation: ddl.OperationCreateTable, Scope: scope, Name: "events", Columns: []ddl.ColumnDefinition{{Name: "id", DataType: "INTEGER", PrimaryKey: true}, {Name: "note", DataType: "text", Nullable: true}}}, want: `CREATE TABLE "tenant""one"."events" ("id" integer NOT NULL, "note" text, PRIMARY KEY ("id"))`},
		{name: "drop table cascade", req: ddl.Request{Operation: ddl.OperationDropObject, Ref: &table, Cascade: true}, want: `DROP TABLE "tenant""one"."order""items" CASCADE`},
		{name: "rename column", req: ddl.Request{Operation: ddl.OperationRenameColumn, Ref: &table, Name: "old", NewName: `new"name`}, want: `ALTER TABLE "tenant""one"."order""items" RENAME COLUMN "old" TO "new""name"`},
		{name: "drop index", req: ddl.Request{Operation: ddl.OperationDropIndex, Ref: &table, Name: "events_idx"}, want: `DROP INDEX "tenant""one"."events_idx"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := postgresDDLSQL(tt.req)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("SQL = %q, want %q", got, tt.want)
			}
		})
	}
}
