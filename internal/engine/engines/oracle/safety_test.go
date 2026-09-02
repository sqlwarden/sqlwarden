package oracle

import (
	"context"
	"testing"

	"github.com/sqlwarden/internal/engine/safety"
)

func TestOracleSafetyCheck(t *testing.T) {
	d := &oracleDriver{}

	res, err := d.Check(context.Background(), safety.Request{SQL: "UPDATE employees SET salary = 0"})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !res.Unsafe || len(res.Statements) != 1 {
		t.Fatalf("want 1 unsafe statement, got %+v", res)
	}
	if res.Statements[0].Kind != safety.KindUnsafeMissingWhere {
		t.Errorf("kind = %v", res.Statements[0].Kind)
	}

	safe, err := d.Check(context.Background(), safety.Request{SQL: "DELETE FROM employees WHERE id = 1"})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if safe.Unsafe {
		t.Errorf("statement with WHERE flagged: %+v", safe)
	}

	broken, err := d.Check(context.Background(), safety.Request{SQL: "DELETE FROM WHERE ("})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if broken.Unsafe {
		t.Errorf("syntax error should yield no findings: %+v", broken)
	}
}
