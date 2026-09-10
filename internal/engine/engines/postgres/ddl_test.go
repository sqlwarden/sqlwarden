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

func TestPostgresDDLSpecAdvertisesEditOperations(t *testing.T) {
	spec := (&Driver{}).DDLSpec()
	for _, op := range []ddl.Operation{ddl.OperationAddColumn, ddl.OperationAlterColumn, ddl.OperationCreateIndex} {
		if !spec.Supports(op) {
			t.Errorf("expected spec to support %q", op)
		}
	}
	if !spec.SupportsColumnDefaults {
		t.Error("expected SupportsColumnDefaults")
	}
}

func TestPostgresDDLSQLAddColumn(t *testing.T) {
	def := "0"
	req := ddl.Request{
		Operation: ddl.OperationAddColumn,
		Ref:       &metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "public"}), Kind: "table", Name: "widgets"},
		Column:    &ddl.ColumnDefinition{Name: "count", DataType: "integer", Nullable: false, Default: &def},
	}
	sql, err := postgresDDLSQL(req)
	if err != nil {
		t.Fatal(err)
	}
	want := `ALTER TABLE "public"."widgets" ADD COLUMN "count" integer DEFAULT (0) NOT NULL`
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
}

func TestPostgresDDLSQLAddColumnParameterizedType(t *testing.T) {
	req := ddl.Request{
		Operation: ddl.OperationAddColumn,
		Ref:       &metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "public"}), Kind: "table", Name: "invoices"},
		Column:    &ddl.ColumnDefinition{Name: "amount", DataType: "numeric(10,2)", Nullable: false},
	}
	sql, err := postgresDDLSQL(req)
	if err != nil {
		t.Fatal(err)
	}
	want := `ALTER TABLE "public"."invoices" ADD COLUMN "amount" numeric(10,2) NOT NULL`
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
}

func TestPostgresDDLSQLCreateTableParameterizedType(t *testing.T) {
	req := ddl.Request{
		Operation: ddl.OperationCreateTable,
		Scope:     metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "public"}),
		Name:      "customers",
		Columns:   []ddl.ColumnDefinition{{Name: "id", DataType: "integer", PrimaryKey: true}, {Name: "name", DataType: "varchar(500)", Nullable: false}},
	}
	sql, err := postgresDDLSQL(req)
	if err != nil {
		t.Fatal(err)
	}
	want := `CREATE TABLE "public"."customers" ("id" integer NOT NULL, "name" varchar(500) NOT NULL, PRIMARY KEY ("id"))`
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
}

func TestPostgresDDLSQLAlterColumn(t *testing.T) {
	newType := "varchar(500)"
	newDefault := "'unknown'"
	nullable := true
	req := ddl.Request{
		Operation: ddl.OperationAlterColumn,
		Ref:       &metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "public"}), Kind: "table", Name: "widgets"},
		Name:      "label",
		Changes:   &ddl.ColumnChanges{DataType: &newType, Default: &newDefault, Nullable: &nullable},
	}
	sql, err := postgresDDLSQL(req)
	if err != nil {
		t.Fatal(err)
	}
	want := `ALTER TABLE "public"."widgets" ALTER COLUMN "label" TYPE varchar(500), ALTER COLUMN "label" SET DEFAULT ('unknown'), ALTER COLUMN "label" DROP NOT NULL`
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
}

func TestPostgresDDLSQLCreateIndex(t *testing.T) {
	req := ddl.Request{
		Operation:    ddl.OperationCreateIndex,
		Ref:          &metadata.ObjectRef{Scope: metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: "public"}), Kind: "table", Name: "widgets"},
		Name:         "widgets_label_idx",
		Unique:       true,
		IndexColumns: []ddl.IndexColumn{{Name: "label"}, {Name: "created_at", Descending: true}},
	}
	sql, err := postgresDDLSQL(req)
	if err != nil {
		t.Fatal(err)
	}
	want := `CREATE UNIQUE INDEX "widgets_label_idx" ON "public"."widgets" ("label", "created_at" DESC)`
	if sql != want {
		t.Errorf("got %q, want %q", sql, want)
	}
}

func TestPostgresParameterizedColumnType(t *testing.T) {
	spec := (&Driver{}).DDLSpec()
	if canonical, ok := spec.CanonicalColumnType("numeric(10,2)"); !ok || canonical != "numeric(10,2)" {
		t.Errorf("got (%q, %v)", canonical, ok)
	}
	if _, ok := spec.CanonicalColumnType("numeric(1001,2)"); ok {
		t.Error("expected precision above range to be rejected")
	}
}
