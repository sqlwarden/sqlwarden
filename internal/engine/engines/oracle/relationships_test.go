package oracle

import (
	"strings"
	"testing"
)

func TestFoldOracleForeignKeys(t *testing.T) {
	scope := oracleSchemaScope("HR")

	t.Run("two-column FK yields one relationship in position order", func(t *testing.T) {
		rows := []oracleForeignKeyRow{
			{Table: "EMP", Name: "EMP_DEPT_FK", Column: "DEPT_ID", RefOwner: "HR", RefTable: "DEPT", RefColumn: "ID"},
			{Table: "EMP", Name: "EMP_DEPT_FK", Column: "DEPT_REGION", RefOwner: "HR", RefTable: "DEPT", RefColumn: "REGION"},
		}
		got := foldOracleForeignKeys(scope, rows)
		if len(got) != 1 {
			t.Fatalf("got %d relationships, want 1", len(got))
		}
		rel := got[0]
		if rel.Name != "EMP_DEPT_FK" || rel.Kind != "foreign_key" {
			t.Fatalf("unexpected relationship head: %+v", rel)
		}
		if got, want := strings.Join(rel.Columns, ","), "DEPT_ID,DEPT_REGION"; got != want {
			t.Fatalf("Columns = %q, want %q", got, want)
		}
		if got, want := strings.Join(rel.ReferencedColumns, ","), "ID,REGION"; got != want {
			t.Fatalf("ReferencedColumns = %q, want %q", got, want)
		}
		if rel.Source.Name != "EMP" || rel.References.Name != "DEPT" || rel.References.Scope.Name("schema") != "HR" {
			t.Fatalf("unexpected endpoints: %+v", rel)
		}
	})

	t.Run("two FKs on the same table yield two relationships", func(t *testing.T) {
		rows := []oracleForeignKeyRow{
			{Table: "EMP", Name: "EMP_DEPT_FK", Column: "DEPT_ID", RefOwner: "HR", RefTable: "DEPT", RefColumn: "ID"},
			{Table: "EMP", Name: "EMP_MGR_FK", Column: "MGR_ID", RefOwner: "HR", RefTable: "EMP", RefColumn: "ID"},
		}
		got := foldOracleForeignKeys(scope, rows)
		if len(got) != 2 {
			t.Fatalf("got %d relationships, want 2", len(got))
		}
		if got[0].Name != "EMP_DEPT_FK" || got[1].Name != "EMP_MGR_FK" {
			t.Fatalf("unexpected names: %q, %q", got[0].Name, got[1].Name)
		}
	})
}

func TestOracleRelationshipsQueryShape(t *testing.T) {
	q := oracleRelationshipsQuery
	for _, want := range []string{
		"all_constraints", "constraint_type = 'R'", "c.owner = :1",
		"r_constraint_name", "ORDER BY",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("relationships query missing %q", want)
		}
	}
}
