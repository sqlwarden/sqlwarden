package mysql

import (
	"testing"

	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
)

func TestMySQLDDLSQL(t *testing.T) {
	scope := metadata.NewScopePath(metadata.ScopeSegment{Kind: "database", Name: "tenant`one"})
	table := metadata.ObjectRef{Scope: scope, Kind: "table", Name: "orders"}
	tests := []struct {
		name string
		req  ddl.Request
		want string
	}{
		{name: "create table", req: ddl.Request{Operation: ddl.OperationCreateTable, Scope: scope, Name: "events", Columns: []ddl.ColumnDefinition{{Name: "id", DataType: "INT", PrimaryKey: true}}}, want: "CREATE TABLE `tenant``one`.`events` (`id` int NOT NULL, PRIMARY KEY (`id`))"},
		{name: "drop database", req: ddl.Request{Operation: ddl.OperationDropScope, Scope: scope}, want: "DROP DATABASE `tenant``one`"},
		{name: "rename column", req: ddl.Request{Operation: ddl.OperationRenameColumn, Ref: &table, Name: "old", NewName: "new`name"}, want: "ALTER TABLE `tenant``one`.`orders` RENAME COLUMN `old` TO `new``name`"},
		{name: "drop index", req: ddl.Request{Operation: ddl.OperationDropIndex, Ref: &table, Name: "orders_idx"}, want: "ALTER TABLE `tenant``one`.`orders` DROP INDEX `orders_idx`"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mysqlDDLSQL(tt.req)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("SQL = %q, want %q", got, tt.want)
			}
		})
	}
}
