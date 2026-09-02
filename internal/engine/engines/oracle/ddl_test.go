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
