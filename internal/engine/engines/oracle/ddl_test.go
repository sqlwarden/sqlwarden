package oracle

import (
	"errors"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/ddl"
	"github.com/sqlwarden/internal/engine/metadata"
)

func TestOracleDDLSQLCreateTable(t *testing.T) {
	got, err := oracleDDLSQL(ddl.Request{
		Operation: ddl.OperationCreateTable,
		Scope:     oracleSchemaScope("HR"),
		Name:      "T",
		Columns: []ddl.ColumnDefinition{
			{Name: "ID", DataType: "number", PrimaryKey: true},
			{Name: "NAME", DataType: "varchar2(255)", Nullable: true},
		},
	})
	if err != nil {
		t.Fatalf("oracleDDLSQL: %v", err)
	}
	want := `CREATE TABLE "HR"."T" ("ID" NUMBER NOT NULL, "NAME" VARCHAR2(255), PRIMARY KEY ("ID"))`
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestOracleDDLSQLDropTableCascade(t *testing.T) {
	ref := &metadata.ObjectRef{Scope: oracleSchemaScope("HR"), Kind: "table", Name: "T"}
	got, err := oracleDDLSQL(ddl.Request{Operation: ddl.OperationDropObject, Ref: ref, Cascade: true})
	if err != nil {
		t.Fatalf("oracleDDLSQL: %v", err)
	}
	if got != `DROP TABLE "HR"."T" CASCADE CONSTRAINTS` {
		t.Fatalf("got %q", got)
	}
}

func TestOracleDDLSQLDropView(t *testing.T) {
	ref := &metadata.ObjectRef{Scope: oracleSchemaScope("HR"), Kind: "view", Name: "V"}
	got, _ := oracleDDLSQL(ddl.Request{Operation: ddl.OperationDropObject, Ref: ref, Cascade: true})
	if got != `DROP VIEW "HR"."V"` {
		t.Fatalf("got %q", got)
	}
}

func TestOracleDDLSQLRenameAndDropColumnAndIndex(t *testing.T) {
	ref := &metadata.ObjectRef{Scope: oracleSchemaScope("HR"), Kind: "table", Name: "T"}
	rename, _ := oracleDDLSQL(ddl.Request{Operation: ddl.OperationRenameColumn, Ref: ref, Name: "A", NewName: "B"})
	if rename != `ALTER TABLE "HR"."T" RENAME COLUMN "A" TO "B"` {
		t.Fatalf("rename: %q", rename)
	}
	dropCol, _ := oracleDDLSQL(ddl.Request{Operation: ddl.OperationDropColumn, Ref: ref, Name: "A"})
	if dropCol != `ALTER TABLE "HR"."T" DROP COLUMN "A"` {
		t.Fatalf("drop column: %q", dropCol)
	}
	idxRef := &metadata.ObjectRef{Scope: oracleSchemaScope("HR"), Kind: "table", Name: "T"}
	dropIdx, _ := oracleDDLSQL(ddl.Request{Operation: ddl.OperationDropIndex, Ref: idxRef, Name: "IX_T_A"})
	if dropIdx != `DROP INDEX "HR"."IX_T_A"` {
		t.Fatalf("drop index: %q", dropIdx)
	}
}

func TestOracleDDLSQLUnsupported(t *testing.T) {
	if _, err := oracleDDLSQL(ddl.Request{Operation: ddl.OperationDropScope, Scope: oracleSchemaScope("HR")}); !errors.Is(err, ddl.ErrUnsupported) {
		t.Fatalf("want ddl.ErrUnsupported, got %v", err)
	}
}

func TestOracleDDLSpecExcludesDropScope(t *testing.T) {
	spec := (&oracleDriver{}).DDLSpec()
	for _, op := range spec.Operations {
		if op == ddl.OperationDropScope {
			t.Fatal("oracle DDL spec must not advertise OperationDropScope")
		}
	}
	if len(spec.CreatableTableScopeKinds) != 1 || spec.CreatableTableScopeKinds[0] != "schema" {
		t.Fatalf("CreatableTableScopeKinds = %v", spec.CreatableTableScopeKinds)
	}
	if !strings.Contains(strings.Join(spec.ColumnTypes, ","), "VARCHAR2(255)") {
		t.Fatalf("ColumnTypes missing VARCHAR2(255): %v", spec.ColumnTypes)
	}
}

func ddlTestPointer[T any](value T) *T { return &value }

func TestOracleParameterizedTypes(t *testing.T) {
	for input, want := range map[string]string{
		"number(10, 2)": "NUMBER(10,2)", "NUMBER(38,-84)": "NUMBER(38,-84)",
		"varchar2(120)": "VARCHAR2(120)", "CHAR(10)": "CHAR(10)", "RAW(64)": "RAW(64)",
		"timestamp(6) with local time zone": "TIMESTAMP(6) WITH LOCAL TIME ZONE",
	} {
		got, ok := oracleDDLSpec.CanonicalColumnType(input)
		if !ok || got != want {
			t.Errorf("%q = %q, %v; want %q", input, got, ok, want)
		}
	}
	for _, input := range []string{"NUMBER(0)", "NUMBER(39)", "NUMBER(10,128)", "NUMBER(10,-85)", "VARCHAR2(4001)", "NUMBER(10,2,3)", "NUMBER(10); DROP TABLE x", "NUMBER(10) NOT NULL", "TIMESTAMP(10)", "VARCHAR2(x)", "NUMBER()"} {
		if _, ok := oracleDDLSpec.CanonicalColumnType(input); ok {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestOracleDefaultExpressionBoundary(t *testing.T) {
	for _, value := range []string{"0", "-1.5", "'O''Brien'", "q'[); DROP TABLE x; --]'", "N'hello'", "SYSDATE", "SYSTIMESTAMP", "SYS_GUID()", "TO_DATE('2026-01-01', 'YYYY-MM-DD')", "(1 + 2) * 3", "NULL"} {
		if err := validateOracleDefault(value); err != nil {
			t.Errorf("valid %q: %v", value, err)
		}
	}
	for _, value := range []string{"", "0); DROP TABLE x; --", "0) NOT NULL, hacked NUMBER DEFAULT (0", "0, 1", "1 -- comment", "1 /* comment */", ":bind", "(SELECT 1 FROM DUAL)", "'unterminated", "q'[unterminated", "0) FROM DUAL UNION SELECT (1"} {
		if err := validateOracleDefault(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
}

func TestOracleTableEditSQL(t *testing.T) {
	ref := &metadata.ObjectRef{Scope: oracleSchemaScope("HR"), Kind: "table", Name: `Order"Items`}
	tests := []struct {
		request ddl.Request
		want    string
	}{
		{ddl.Request{Operation: ddl.OperationAddColumn, Ref: ref, Column: &ddl.ColumnDefinition{Name: "AMOUNT", DataType: "number(10,2)", Nullable: false, Default: ddlTestPointer("0")}}, `ALTER TABLE "HR"."Order""Items" ADD ("AMOUNT" NUMBER(10,2) DEFAULT (0) NOT NULL)`},
		{ddl.Request{Operation: ddl.OperationAlterColumn, Ref: ref, Name: "AMOUNT", Changes: &ddl.ColumnChanges{DataType: ddlTestPointer("number(12,2)")}}, `ALTER TABLE "HR"."Order""Items" MODIFY ("AMOUNT" NUMBER(12,2))`},
		{ddl.Request{Operation: ddl.OperationAlterColumn, Ref: ref, Name: "AMOUNT", Changes: &ddl.ColumnChanges{Nullable: ddlTestPointer(true), Default: ddlTestPointer("NULL")}}, `ALTER TABLE "HR"."Order""Items" MODIFY ("AMOUNT" DEFAULT (NULL) NULL)`},
		{ddl.Request{Operation: ddl.OperationCreateIndex, Ref: ref, Name: "IX_ITEMS", Unique: true, IndexColumns: []ddl.IndexColumn{{Name: "AMOUNT", Descending: true}, {Name: `A"B`}}}, `CREATE UNIQUE INDEX "HR"."IX_ITEMS" ON "HR"."Order""Items" ("AMOUNT" DESC, "A""B")`},
	}
	for _, tt := range tests {
		got, err := oracleDDLSQL(tt.request)
		if err != nil || got != tt.want {
			t.Errorf("%s: got %q, %v; want %q", tt.request.Operation, got, err, tt.want)
		}
	}
}

func TestOracleRejectsInvalidTableEdits(t *testing.T) {
	ref := &metadata.ObjectRef{Scope: oracleSchemaScope("HR"), Kind: "table", Name: "T"}
	requests := []ddl.Request{
		{Operation: ddl.OperationAddColumn, Ref: ref},
		{Operation: ddl.OperationAddColumn, Ref: ref, Column: &ddl.ColumnDefinition{Name: "A", DataType: "NUMBER(500)"}},
		{Operation: ddl.OperationAddColumn, Ref: ref, Column: &ddl.ColumnDefinition{Name: "A", DataType: "NUMBER", Default: ddlTestPointer("1) NOT NULL, B NUMBER DEFAULT (0")}},
		{Operation: ddl.OperationAlterColumn, Ref: ref, Name: "A", Changes: &ddl.ColumnChanges{}},
		{Operation: ddl.OperationCreateIndex, Ref: ref, Name: "IX"},
		{Operation: ddl.OperationCreateIndex, Ref: ref, Name: "IX", IndexColumns: []ddl.IndexColumn{{Name: "A"}, {Name: "A"}}},
		{Operation: ddl.OperationAlterColumn, Ref: &metadata.ObjectRef{Scope: ref.Scope, Kind: "view", Name: "V"}, Name: "A", Changes: &ddl.ColumnChanges{Nullable: ddlTestPointer(false)}},
	}
	for _, request := range requests {
		if _, err := oracleDDLSQL(request); err == nil {
			t.Errorf("accepted %+v", request)
		}
	}
}
