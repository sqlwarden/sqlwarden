package oracle

import (
	"strings"
	"testing"

	"github.com/sqlwarden/internal/engine/statement"
)

func TestOracleGenerateSelect(t *testing.T) {
	got, err := (&oracleDriver{}).Generate(statement.Request{Operation: statement.OperationSelect, Object: oracleTestTable()})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(got, `"HR"."EMPLOYEES"`) || !strings.Contains(got, `"ID"`) {
		t.Fatalf("unexpected SELECT: %q", got)
	}
}

func TestOracleGenerateInsert(t *testing.T) {
	got, err := (&oracleDriver{}).Generate(statement.Request{Operation: statement.OperationInsert, Object: oracleTestTable()})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(got, `INSERT INTO "HR"."EMPLOYEES"`) || !strings.Contains(got, ":1") || !strings.Contains(got, ":2") {
		t.Fatalf("unexpected INSERT: %q", got)
	}
}

func TestOracleGenerateUpdateDeleteGuarded(t *testing.T) {
	for _, op := range []statement.Operation{statement.OperationUpdate, statement.OperationDelete} {
		got, err := (&oracleDriver{}).Generate(statement.Request{Operation: op, Object: oracleTestTable()})
		if err != nil {
			t.Fatalf("Generate(%v): %v", op, err)
		}
		if !strings.Contains(got, "WHERE 1 = 0") {
			t.Fatalf("op %v missing guard: %q", op, got)
		}
	}
}

func TestOracleStatementSpec(t *testing.T) {
	spec := (&oracleDriver{}).StatementSpec()
	kinds := map[string]bool{}
	for _, o := range spec.Objects {
		kinds[o.Kind] = true
	}
	for _, want := range []string{"table", "view", "materialized_view"} {
		if !kinds[want] {
			t.Errorf("spec missing kind %q", want)
		}
	}
}
